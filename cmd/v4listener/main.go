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

// ---- Minimal ABI: all PoolManager + ERC-6909 events ----
const poolManagerEventsABI = `[
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true,  "internalType": "bytes32", "name": "id",           "type": "bytes32"},
      {"indexed": true,  "internalType": "address", "name": "currency0",    "type": "address"},
      {"indexed": true,  "internalType": "address", "name": "currency1",    "type": "address"},
      {"indexed": false, "internalType": "uint24",  "name": "fee",          "type": "uint24"},
      {"indexed": false, "internalType": "int24",   "name": "tickSpacing",  "type": "int24"},
      {"indexed": false, "internalType": "address", "name": "hooks",        "type": "address"},
      {"indexed": false, "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160"},
      {"indexed": false, "internalType": "int24",   "name": "tick",         "type": "int24"}
    ],
    "name": "Initialize",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true,  "internalType": "bytes32", "name": "id",             "type": "bytes32"},
      {"indexed": true,  "internalType": "address", "name": "sender",         "type": "address"},
      {"indexed": false, "internalType": "int24",   "name": "tickLower",      "type": "int24"},
      {"indexed": false, "internalType": "int24",   "name": "tickUpper",      "type": "int24"},
      {"indexed": false, "internalType": "int256",  "name": "liquidityDelta", "type": "int256"},
      {"indexed": false, "internalType": "bytes32", "name": "salt",           "type": "bytes32"}
    ],
    "name": "ModifyLiquidity",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true,  "internalType": "bytes32", "name": "id",           "type": "bytes32"},
      {"indexed": true,  "internalType": "address", "name": "sender",       "type": "address"},
      {"indexed": false, "internalType": "int128",  "name": "amount0",      "type": "int128"},
      {"indexed": false, "internalType": "int128",  "name": "amount1",      "type": "int128"},
      {"indexed": false, "internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160"},
      {"indexed": false, "internalType": "uint128", "name": "liquidity",    "type": "uint128"},
      {"indexed": false, "internalType": "int24",   "name": "tick",         "type": "int24"},
      {"indexed": false, "internalType": "uint24",  "name": "fee",          "type": "uint24"}
    ],
    "name": "Swap",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true,  "internalType": "bytes32", "name": "id",      "type": "bytes32"},
      {"indexed": true,  "internalType": "address", "name": "sender",  "type": "address"},
      {"indexed": false, "internalType": "uint256", "name": "amount0", "type": "uint256"},
      {"indexed": false, "internalType": "uint256", "name": "amount1", "type": "uint256"}
    ],
    "name": "Donate",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true,  "internalType": "address", "name": "owner",    "type": "address"},
      {"indexed": true,  "internalType": "address", "name": "operator", "type": "address"},
      {"indexed": false, "internalType": "bool",    "name": "approved", "type": "bool"}
    ],
    "name": "OperatorSet",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": false, "internalType": "address", "name": "caller", "type": "address"},
      {"indexed": true,  "internalType": "address", "name": "from",   "type": "address"},
      {"indexed": true,  "internalType": "address", "name": "to",     "type": "address"},
      {"indexed": true,  "internalType": "uint256", "name": "id",     "type": "uint256"},
      {"indexed": false, "internalType": "uint256", "name": "amount", "type": "uint256"}
    ],
    "name": "Transfer",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true,  "internalType": "bytes32", "name": "id",          "type": "bytes32"},
      {"indexed": false, "internalType": "uint24",  "name": "protocolFee", "type": "uint24"}
    ],
    "name": "ProtocolFeeUpdated",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true, "internalType": "address", "name": "protocolFeeController", "type": "address"}
    ],
    "name": "ProtocolFeeControllerUpdated",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {"indexed": true, "internalType": "address", "name": "previousOwner", "type": "address"},
      {"indexed": true, "internalType": "address", "name": "newOwner",      "type": "address"}
    ],
    "name": "OwnershipTransferred",
    "type": "event"
  }
]`

// ---- Types ----

type PoolKey struct {
	Currency0   common.Address
	Currency1   common.Address
	Fee         uint32 // uint24 fits
	TickSpacing int32  // int24 fits
	Hooks       common.Address
}

type InitializeEvent struct {
	// indexed
	Id        common.Hash
	Currency0 common.Address
	Currency1 common.Address
	// non-indexed
	Fee          *big.Int // uint24 → *big.Int in go-ethereum
	TickSpacing  *big.Int // int24 → *big.Int in go-ethereum
	Hooks        common.Address
	SqrtPriceX96 *big.Int
	Tick         *big.Int // int24 → *big.Int in go-ethereum
}

type ModifyLiquidityEvent struct {
	// indexed
	Id     common.Hash
	Sender common.Address
	// non-indexed
	TickLower      *big.Int // int24 → *big.Int in go-ethereum
	TickUpper      *big.Int // int24 → *big.Int in go-ethereum
	LiquidityDelta *big.Int
	Salt           [32]byte
}

type SwapEvent struct {
	// indexed
	Id     common.Hash
	Sender common.Address
	// non-indexed
	Amount0      *big.Int // int128
	Amount1      *big.Int
	SqrtPriceX96 *big.Int // uint160
	Liquidity    *big.Int // uint128
	Tick         *big.Int // int24 → *big.Int in go-ethereum
	Fee          *big.Int // uint24 → *big.Int in go-ethereum
}

type DonateEvent struct {
	// indexed
	Id     common.Hash
	Sender common.Address
	// non-indexed
	Amount0 *big.Int
	Amount1 *big.Int
}

type OperatorSetEvent struct {
	// indexed
	Owner    common.Address
	Operator common.Address
	// non-indexed
	Approved bool
}

type TransferEvent struct {
	// indexed
	From common.Address
	To   common.Address
	Id   *big.Int // uint256 token id
	// non-indexed
	Caller common.Address
	Amount *big.Int
}

type ProtocolFeeUpdatedEvent struct {
	// indexed
	Id common.Hash
	// non-indexed
	ProtocolFee *big.Int // uint24 → *big.Int in go-ethereum
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

func signedBigIntToString(x *big.Int) string {
	if x == nil {
		return "nil"
	}
	return x.String()
}

// currencyLabel prints "NATIVE" for address(0) (v4 native ETH representation).
func currencyLabel(a common.Address) string {
	if (a == common.Address{}) {
		return "NATIVE"
	}
	return a.Hex()
}

// resolvePool looks up a pool's display pair and hooks strings from the local cache.
func resolvePool(mu *sync.RWMutex, poolKeys map[common.Hash]PoolKey, id common.Hash) (pair, hooks string) {
	mu.RLock()
	key, ok := poolKeys[id]
	mu.RUnlock()
	if !ok {
		return "UNKNOWN/UNKNOWN", "UNKNOWN"
	}
	if (key.Currency0 == common.Address{}) || (key.Currency1 == common.Address{}) {
		pair = fmt.Sprintf("%s/%s", currencyLabel(key.Currency0), currencyLabel(key.Currency1))
	} else {
		pair = fmt.Sprintf("%s/%s", shortAddr(key.Currency0), shortAddr(key.Currency1))
	}
	return pair, shortAddr(key.Hooks)
}

// ---- Per-event handlers ----

func handleInitialize(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]PoolKey) error {
	if len(lg.Topics) < 4 {
		return fmt.Errorf("Initialize: want 4 topics, got %d", len(lg.Topics))
	}
	var ev InitializeEvent
	ev.Id = lg.Topics[1]
	ev.Currency0 = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	ev.Currency1 = common.BytesToAddress(lg.Topics[3].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "Initialize", lg.Data); err != nil {
		return fmt.Errorf("unpack Initialize: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	mu.Lock()
	poolKeys[ev.Id] = PoolKey{
		Currency0:   ev.Currency0,
		Currency1:   ev.Currency1,
		Fee:         uint32(ev.Fee.Uint64()),
		TickSpacing: int32(ev.TickSpacing.Int64()),
		Hooks:       ev.Hooks,
	}
	mu.Unlock()
	fmt.Printf(
		"[Initialize]         block=%d tx=%s poolId=%s c0=%s c1=%s fee=%s tickSpacing=%s hooks=%s sqrtPrice=%s tick=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortHash(ev.Id),
		currencyLabel(ev.Currency0), currencyLabel(ev.Currency1),
		ev.Fee.String(), ev.TickSpacing.String(), shortAddr(ev.Hooks),
		ev.SqrtPriceX96.String(), ev.Tick.String(),
	)
	return nil
}

func handleModifyLiquidity(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]PoolKey) error {
	if len(lg.Topics) < 3 {
		return fmt.Errorf("ModifyLiquidity: want 3 topics, got %d", len(lg.Topics))
	}
	var ev ModifyLiquidityEvent
	ev.Id = lg.Topics[1]
	ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "ModifyLiquidity", lg.Data); err != nil {
		return fmt.Errorf("unpack ModifyLiquidity: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	pair, hooks := resolvePool(mu, poolKeys, ev.Id)
	action := "add"
	if ev.LiquidityDelta != nil && ev.LiquidityDelta.Sign() < 0 {
		action = "remove"
	}
	fmt.Printf(
		"[ModifyLiquidity]    block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s action=%s delta=%s tickLow=%s tickHigh=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortHash(ev.Id),
		pair, hooks, shortAddr(ev.Sender),
		action, signedBigIntToString(ev.LiquidityDelta),
		signedBigIntToString(ev.TickLower), signedBigIntToString(ev.TickUpper),
	)
	return nil
}

func handleSwap(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]PoolKey) error {
	if len(lg.Topics) < 3 {
		return fmt.Errorf("Swap: want 3 topics, got %d", len(lg.Topics))
	}
	var ev SwapEvent
	ev.Id = lg.Topics[1]
	ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "Swap", lg.Data); err != nil {
		return fmt.Errorf("unpack Swap: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	pair, hooks := resolvePool(mu, poolKeys, ev.Id)
	dirHint := ""
	if ev.Amount0 != nil && ev.Amount0.Sign() < 0 {
		dirHint = " (token0 out)"
	} else if ev.Amount1 != nil && ev.Amount1.Sign() < 0 {
		dirHint = " (token1 out)"
	}
	fmt.Printf(
		"[Swap]               block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s amt0=%s amt1=%s tick=%s fee=%s%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortHash(ev.Id),
		pair, hooks, shortAddr(ev.Sender),
		signedBigIntToString(ev.Amount0), signedBigIntToString(ev.Amount1),
		signedBigIntToString(ev.Tick), ev.Fee.String(), dirHint,
	)
	return nil
}

func handleDonate(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]PoolKey) error {
	if len(lg.Topics) < 3 {
		return fmt.Errorf("Donate: want 3 topics, got %d", len(lg.Topics))
	}
	var ev DonateEvent
	ev.Id = lg.Topics[1]
	ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "Donate", lg.Data); err != nil {
		return fmt.Errorf("unpack Donate: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	pair, hooks := resolvePool(mu, poolKeys, ev.Id)
	fmt.Printf(
		"[Donate]             block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s amt0=%s amt1=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortHash(ev.Id),
		pair, hooks, shortAddr(ev.Sender),
		ev.Amount0.String(), ev.Amount1.String(),
	)
	return nil
}

func handleOperatorSet(parsedABI *abi.ABI, lg types.Log) error {
	if len(lg.Topics) < 3 {
		return fmt.Errorf("OperatorSet: want 3 topics, got %d", len(lg.Topics))
	}
	var ev OperatorSetEvent
	ev.Owner = common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	ev.Operator = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "OperatorSet", lg.Data); err != nil {
		return fmt.Errorf("unpack OperatorSet: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	fmt.Printf(
		"[OperatorSet]        block=%d tx=%s owner=%s operator=%s approved=%v\n",
		lg.BlockNumber, shortHash(lg.TxHash),
		shortAddr(ev.Owner), shortAddr(ev.Operator), ev.Approved,
	)
	return nil
}

func handleTransfer(parsedABI *abi.ABI, lg types.Log) error {
	if len(lg.Topics) < 4 {
		return fmt.Errorf("Transfer: want 4 topics, got %d", len(lg.Topics))
	}
	var ev TransferEvent
	ev.From = common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	ev.To = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	ev.Id = new(big.Int).SetBytes(lg.Topics[3].Bytes())
	if err := parsedABI.UnpackIntoInterface(&ev, "Transfer", lg.Data); err != nil {
		return fmt.Errorf("unpack Transfer: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	fmt.Printf(
		"[Transfer]           block=%d tx=%s from=%s to=%s tokenId=%s amount=%s caller=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash),
		shortAddr(ev.From), shortAddr(ev.To),
		ev.Id.String(), ev.Amount.String(), shortAddr(ev.Caller),
	)
	return nil
}

func handleProtocolFeeUpdated(parsedABI *abi.ABI, lg types.Log) error {
	if len(lg.Topics) < 2 {
		return fmt.Errorf("ProtocolFeeUpdated: want 2 topics, got %d", len(lg.Topics))
	}
	var ev ProtocolFeeUpdatedEvent
	ev.Id = lg.Topics[1]
	if err := parsedABI.UnpackIntoInterface(&ev, "ProtocolFeeUpdated", lg.Data); err != nil {
		return fmt.Errorf("unpack ProtocolFeeUpdated: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	fmt.Printf(
		"[ProtocolFeeUpdated] block=%d tx=%s poolId=%s protocolFee=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortHash(ev.Id), ev.ProtocolFee.String(),
	)
	return nil
}

func handleProtocolFeeControllerUpdated(lg types.Log) error {
	if len(lg.Topics) < 2 {
		return fmt.Errorf("ProtocolFeeControllerUpdated: want 2 topics, got %d", len(lg.Topics))
	}
	controller := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	fmt.Printf(
		"[FeeControllerUpd]   block=%d tx=%s newController=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortAddr(controller),
	)
	return nil
}

func handleOwnershipTransferred(lg types.Log) error {
	if len(lg.Topics) < 3 {
		return fmt.Errorf("OwnershipTransferred: want 3 topics, got %d", len(lg.Topics))
	}
	prev := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	next := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	fmt.Printf(
		"[OwnershipXfer]      block=%d tx=%s from=%s to=%s\n",
		lg.BlockNumber, shortHash(lg.TxHash), shortAddr(prev), shortAddr(next),
	)
	return nil
}

// ---- Dispatcher ----

func handleLog(
	parsedABI *abi.ABI,
	lg types.Log,
	eventNames map[common.Hash]string,
	mu *sync.RWMutex,
	poolKeys map[common.Hash]PoolKey,
) error {
	if len(lg.Topics) == 0 {
		return nil
	}
	name, ok := eventNames[lg.Topics[0]]
	if !ok {
		return nil
	}
	switch name {
	case "Initialize":
		return handleInitialize(parsedABI, lg, mu, poolKeys)
	case "ModifyLiquidity":
		return handleModifyLiquidity(parsedABI, lg, mu, poolKeys)
	case "Swap":
		return handleSwap(parsedABI, lg, mu, poolKeys)
	case "Donate":
		return handleDonate(parsedABI, lg, mu, poolKeys)
	case "OperatorSet":
		return handleOperatorSet(parsedABI, lg)
	case "Transfer":
		return handleTransfer(parsedABI, lg)
	case "ProtocolFeeUpdated":
		return handleProtocolFeeUpdated(parsedABI, lg)
	case "ProtocolFeeControllerUpdated":
		return handleProtocolFeeControllerUpdated(lg)
	case "OwnershipTransferred":
		return handleOwnershipTransferred(lg)
	}
	return nil
}

// ---- Entry point ----

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

	// Build event name lookup and collect all topic signatures.
	eventNames := make(map[common.Hash]string, len(parsedABI.Events))
	allSigs := make([]common.Hash, 0, len(parsedABI.Events))
	for name, ev := range parsedABI.Events {
		eventNames[ev.ID] = name
		allSigs = append(allSigs, ev.ID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := ethclient.DialContext(ctx, *wsURL)
	if err != nil {
		log.Fatalf("dial ws: %v", err)
	}
	defer client.Close()

	var (
		mu       sync.RWMutex
		poolKeys = make(map[common.Hash]PoolKey)
	)

	// Optionally backfill Initialize events to pre-populate the pool key cache.
	if *fromBlock > 0 {
		fmt.Printf("Backfilling Initialize events from block %d...\n", *fromBlock)
		q := ethereum.FilterQuery{
			FromBlock: big.NewInt(*fromBlock),
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{parsedABI.Events["Initialize"].ID}},
		}
		logs, err := client.FilterLogs(ctx, q)
		if err != nil {
			log.Fatalf("FilterLogs backfill: %v", err)
		}
		for _, lg := range logs {
			if err := handleLog(&parsedABI, lg, eventNames, &mu, poolKeys); err != nil {
				log.Printf("backfill decode error: %v", err)
			}
		}
		fmt.Printf("Backfill done. Cached pools: %d\n", len(poolKeys))
	} else {
		fmt.Println("No backfill (-from not set). Pool names will only resolve for pools initialized after startup.")
	}

	// Live subscription to all events.
	query := ethereum.FilterQuery{
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{allSigs},
	}

	logCh := make(chan types.Log, 1024)
	sub, err := client.SubscribeFilterLogs(ctx, query, logCh)
	if err != nil {
		log.Fatalf("SubscribeFilterLogs: %v", err)
	}
	fmt.Printf("Listening… PoolManager=%s\n", poolManager.Hex())

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
			if err := handleLog(&parsedABI, lg, eventNames, &mu, poolKeys); err != nil {
				log.Printf("decode error: %v", err)
			}
		}
	}
}
