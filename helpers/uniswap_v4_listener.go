package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// ---- Minimal ABI: only the events we care about ----
//
// NOTE: PoolKey is a tuple in the event. ABI-encoding for tuples in events works
// fine with go-ethereum abi.UnpackIntoInterface when you model it as a struct.
const poolManagerEventsABI = `[
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true, "internalType": "bytes32", "name": "id", "type": "bytes32"},
      {"indexed": false, "internalType": "tuple", "name": "key", "type": "tuple",
        "components": [
          {"internalType":"address","name":"currency0","type":"address"},
          {"internalType":"address","name":"currency1","type":"address"},
          {"internalType":"uint24","name":"fee","type":"uint24"},
          {"internalType":"int24","name":"tickSpacing","type":"int24"},
          {"internalType":"address","name":"hooks","type":"address"}
        ]
      }
    ],
    "name": "PoolCreated",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true, "internalType": "bytes32", "name": "id", "type": "bytes32"},
      {"indexed": true, "internalType": "address", "name": "sender", "type": "address"},
      {"indexed": false, "internalType": "int128", "name": "amount0", "type": "int128"},
      {"indexed": false, "internalType": "int128", "name": "amount1", "type": "int128"},
      {"indexed": false, "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160"},
      {"indexed": false, "internalType": "uint128", "name": "liquidity", "type": "uint128"},
      {"indexed": false, "internalType": "int24", "name": "tick", "type": "int24"},
      {"indexed": false, "internalType": "uint24", "name": "fee", "type": "uint24"}
    ],
    "name": "Swap",
    "type": "event"
  }
]`

// ---- Types to receive decoded event data ----

type PoolKey struct {
	Currency0   common.Address
	Currency1   common.Address
	Fee         uint32 // uint24 fits
	TickSpacing int32  // int24 fits
	Hooks       common.Address
}

type PoolCreatedEvent struct {
	// indexed
	Id common.Hash
	// non-indexed
	Key PoolKey
}

type SwapEvent struct {
	// indexed
	Id     common.Hash
	Sender common.Address

	// non-indexed
	Amount0      *big.Int // int128 => big.Int
	Amount1      *big.Int
	SqrtPriceX96 *big.Int // uint160 => big.Int
	Liquidity    *big.Int // uint128 => big.Int
	Tick         int32
	Fee          uint32 // uint24 fits
}

// ---- Helpers ----

func shortAddr(a common.Address) string {
	s := a.Hex()
	if len(s) <= 10 {
		return s
	}
	return s[:6] + "…" + s[len(s)-4:]
}

func shortHash(h common.Hash) string {
	s := h.Hex()
	return s[:10] + "…" + s[len(s)-6:]
}

// signedBigIntToString prints with sign, and also tells you direction-ish.
func signedBigIntToString(x *big.Int) string {
	if x == nil {
		return "nil"
	}
	return x.String()
}

// For v4, "native" currency can be represented by address(0) in some stacks.
// This prints "NATIVE" if zero.
func currencyLabel(a common.Address) string {
	if (a == common.Address{}) {
		return "NATIVE"
	}
	return a.Hex()
}

func main() {
	var (
		wsURL          = flag.String("ws", "", "WebSocket RPC url (required), e.g. wss://mainnet.infura.io/ws/v3/KEY")
		poolManagerHex = flag.String("poolmanager", "", "Uniswap v4 PoolManager address (required)")
		fromBlock      = flag.Int64("from", 0, "Optional: start from this block (0 = latest only)")
	)
	flag.Parse()

	if *wsURL == "" || *poolManagerHex == "" {
		log.Fatalf("usage: go run helpers/uniswap_v4_listener.go -ws <wss-url> -poolmanager <address> [-from <block>]")
	}

	poolManager := common.HexToAddress(*poolManagerHex)
	if poolManager == (common.Address{}) {
		log.Fatalf("invalid poolmanager address: %s", *poolManagerHex)
	}

	parsedABI, err := abi.JSON(strings.NewReader(poolManagerEventsABI))
	if err != nil {
		log.Fatalf("parse ABI: %v", err)
	}

	// Event signatures
	poolCreatedSig := parsedABI.Events["PoolCreated"].ID
	swapSig := parsedABI.Events["Swap"].ID

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := ethclient.DialContext(ctx, *wsURL)
	if err != nil {
		log.Fatalf("dial ws: %v", err)
	}
	defer client.Close()

	// Local in-memory registry: PoolId -> PoolKey
	var (
		mu       sync.RWMutex
		poolKeys = make(map[common.Hash]PoolKey)
	)

	// Optionally backfill PoolCreated starting from fromBlock (strongly recommended if you want token resolution).
	// This is a one-shot log query; then we start live subscription.
	if *fromBlock > 0 {
		fmt.Printf("Backfilling PoolCreated events from block %d...\n", *fromBlock)
		q := ethereum.FilterQuery{
			FromBlock: big.NewInt(*fromBlock),
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{poolCreatedSig}},
		}
		logs, err := client.FilterLogs(ctx, q)
		if err != nil {
			log.Fatalf("FilterLogs backfill: %v", err)
		}
		for _, lg := range logs {
			if err := handleLog(&parsedABI, lg, poolCreatedSig, swapSig, &mu, poolKeys); err != nil {
				log.Printf("backfill decode error: %v", err)
			}
		}
		fmt.Printf("Backfill done. Cached pools: %d\n", len(poolKeys))
	} else {
		fmt.Println("No backfill (-from not set). You will only resolve PoolKeys for pools created after you start this listener.")
	}

	// Live subscription to both PoolCreated and Swap
	query := ethereum.FilterQuery{
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{poolCreatedSig, swapSig}},
	}

	logCh := make(chan types.Log, 1024)
	sub, err := client.SubscribeFilterLogs(ctx, query, logCh)
	if err != nil {
		log.Fatalf("SubscribeFilterLogs: %v", err)
	}
	fmt.Printf("Listening… PoolManager=%s\n", poolManager.Hex())

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("subscription error: %v", err)

		case s := <-sigCh:
			fmt.Printf("\nGot signal %v, shutting down.\n", s)
			return

		case lg := <-logCh:
			if err := handleLog(&parsedABI, lg, poolCreatedSig, swapSig, &mu, poolKeys); err != nil {
				log.Printf("decode error: %v", err)
			}
		}
	}
}

func handleLog(
	parsedABI *abi.ABI,
	lg types.Log,
	poolCreatedSig common.Hash,
	swapSig common.Hash,
	mu *sync.RWMutex,
	poolKeys map[common.Hash]PoolKey,
) error {
	if len(lg.Topics) == 0 {
		return nil
	}
	switch lg.Topics[0] {
	case poolCreatedSig:
		// Topics: [sig, id]
		if len(lg.Topics) < 2 {
			return fmt.Errorf("PoolCreated missing id topic")
		}

		var ev PoolCreatedEvent
		ev.Id = lg.Topics[1]

		// Non-indexed data contains "key" tuple.
		// We unpack into a temporary struct with the right field layout.
		var out struct {
			Key PoolKey
		}
		if err := parsedABI.UnpackIntoInterface(&out, "PoolCreated", lg.Data); err != nil {
			// Helpful extra debug if ABI mismatch:
			return fmt.Errorf("unpack PoolCreated: %w (data=%s)", err, hex.EncodeToString(lg.Data))
		}
		ev.Key = out.Key

		mu.Lock()
		poolKeys[ev.Id] = ev.Key
		mu.Unlock()

		fmt.Printf(
			"[PoolCreated] block=%d tx=%s poolId=%s c0=%s c1=%s fee=%d tickSpacing=%d hooks=%s\n",
			lg.BlockNumber,
			shortHash(lg.TxHash),
			shortHash(ev.Id),
			currencyLabel(ev.Key.Currency0),
			currencyLabel(ev.Key.Currency1),
			ev.Key.Fee,
			ev.Key.TickSpacing,
			ev.Key.Hooks.Hex(),
		)

	case swapSig:
		// Topics: [sig, id, sender]
		if len(lg.Topics) < 3 {
			return fmt.Errorf("Swap missing indexed topics")
		}

		var ev SwapEvent
		ev.Id = lg.Topics[1]
		ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:]) // last 20 bytes

		// Unpack non-indexed fields
		if err := parsedABI.UnpackIntoInterface(&ev, "Swap", lg.Data); err != nil {
			return fmt.Errorf("unpack Swap: %w (data=%s)", err, hex.EncodeToString(lg.Data))
		}

		// Resolve PoolKey if cached
		mu.RLock()
		key, ok := poolKeys[ev.Id]
		mu.RUnlock()

		pair := "UNKNOWN/UNKNOWN"
		hooks := "UNKNOWN"
		if ok {
			pair = fmt.Sprintf("%s/%s", shortAddr(key.Currency0), shortAddr(key.Currency1))
			if (key.Currency0 == common.Address{}) || (key.Currency1 == common.Address{}) {
				pair = fmt.Sprintf("%s/%s", currencyLabel(key.Currency0), currencyLabel(key.Currency1))
			}
			hooks = shortAddr(key.Hooks)
		}

		// Direction hint: if amount0 < 0, pool paid out token0 and received token1 (typical sign convention).
		dirHint := ""
		if ev.Amount0 != nil && ev.Amount0.Sign() < 0 {
			dirHint = " (token0 out)"
		} else if ev.Amount1 != nil && ev.Amount1.Sign() < 0 {
			dirHint = " (token1 out)"
		}

		fmt.Printf(
			"[Swap]       block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s amt0=%s amt1=%s tick=%d fee=%d%s\n",
			lg.BlockNumber,
			shortHash(lg.TxHash),
			shortHash(ev.Id),
			pair,
			hooks,
			shortAddr(ev.Sender),
			signedBigIntToString(ev.Amount0),
			signedBigIntToString(ev.Amount1),
			ev.Tick,
			ev.Fee,
			dirHint,
		)

	default:
		// ignore
	}
	return nil
}
