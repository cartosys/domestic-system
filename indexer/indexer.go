package indexer

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"charm-wallet-tui/rpc"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	transferSig = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	v4SwapSig   = crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)"))
)

const pollInterval     = 12 * time.Second
const backChunkSize    = uint64(500)
const backChunkInterval = 2 * time.Second
const v4PoolManagerAddress = "0x000000000004444c5dc75cB358380D2e3dE08A90"

const v4SwapEventABI = `[{
	"anonymous": false,
	"inputs": [
		{"indexed": true,  "name": "id",          "type": "bytes32"},
		{"indexed": true,  "name": "sender",       "type": "address"},
		{"indexed": false, "name": "amount0",      "type": "int128"},
		{"indexed": false, "name": "amount1",      "type": "int128"},
		{"indexed": false, "name": "sqrtPriceX96", "type": "uint160"},
		{"indexed": false, "name": "liquidity",    "type": "uint128"},
		{"indexed": false, "name": "tick",         "type": "int24"},
		{"indexed": false, "name": "fee",          "type": "uint24"}
	],
	"name": "Swap",
	"type": "event"
}]`

// IndexedEvent represents a detected ERC-20 Transfer event involving a watched address.
type IndexedEvent struct {
	Block    uint64
	TxHash   common.Hash
	LogIndex uint
	From     common.Address
	To       common.Address
	Value    *big.Int
	Token    common.Address
	Symbol   string
	Decimals uint8
}

// V4SwapEvent represents a Uniswap V4 Swap event where the sender is a watched address.
type V4SwapEvent struct {
	Block        uint64
	TxHash       common.Hash
	LogIndex     uint
	PoolID       common.Hash
	Sender       common.Address
	Amount0      *big.Int
	Amount1      *big.Int
	SqrtPriceX96 *big.Int
	Liquidity    *big.Int
	Tick         *big.Int
	Fee          *big.Int
}

// Indexer polls for ERC-20 Transfer events and Uniswap V4 Swap events involving saved wallet addresses.
type Indexer struct {
	events  chan IndexedEvent
	v4swaps chan V4SwapEvent
	cancel  context.CancelFunc
	mu      sync.Mutex
	cursor  uint64
}

// New creates a new Indexer. Call Start to begin indexing.
func New() *Indexer {
	return &Indexer{
		events:  make(chan IndexedEvent, 256),
		v4swaps: make(chan V4SwapEvent, 256),
	}
}

// Start begins background polling. Non-blocking.
func (idx *Indexer) Start(rpcURL string, addrs []common.Address, tokens []rpc.WatchedToken) {
	ctx, cancel := context.WithCancel(context.Background())
	idx.cancel = cancel
	go idx.run(ctx, rpcURL, addrs, tokens)
}

// Stop halts the indexer and closes the events channel.
func (idx *Indexer) Stop() {
	if idx.cancel != nil {
		idx.cancel()
	}
}

// Events returns the read-only channel of indexed ERC-20 Transfer events.
func (idx *Indexer) Events() <-chan IndexedEvent {
	return idx.events
}

// V4Swaps returns the read-only channel of indexed Uniswap V4 Swap events.
func (idx *Indexer) V4Swaps() <-chan V4SwapEvent {
	return idx.v4swaps
}

func (idx *Indexer) run(ctx context.Context, rpcURL string, addrs []common.Address, tokens []rpc.WatchedToken) {
	// runCtx is cancelled when run() exits for any reason, stopping sub-goroutines.
	runCtx, runCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		runCancel()
		wg.Wait()
		close(idx.events)
		close(idx.v4swaps)
	}()

	dialCtx, dialCancel := context.WithTimeout(ctx, 8*time.Second)
	client, err := ethclient.DialContext(dialCtx, rpcURL)
	dialCancel()
	if err != nil {
		return
	}
	defer client.Close()

	swapABI, err := abi.JSON(strings.NewReader(v4SwapEventABI))
	if err != nil {
		return
	}

	// Build token contract address list and lookup map.
	tokenAddrs := make([]common.Address, len(tokens))
	tokenByAddr := make(map[common.Address]rpc.WatchedToken, len(tokens))
	for i, t := range tokens {
		tokenAddrs[i] = t.Address
		tokenByAddr[t.Address] = t
	}

	// Pad watched addresses into topic hashes for FilterLogs.
	watchedTopics := make([]common.Hash, len(addrs))
	for i, a := range addrs {
		watchedTopics[i] = common.BytesToHash(a.Bytes())
	}

	// Get current tip. Forward polling covers tip+1 onwards; backward scan covers tip downward.
	tipCtx, tipCancel := context.WithTimeout(ctx, 8*time.Second)
	tip, err := client.BlockNumber(tipCtx)
	tipCancel()
	if err != nil {
		return
	}
	idx.mu.Lock()
	idx.cursor = tip
	idx.mu.Unlock()

	// Backward scanner: scans from tip down to block 0 in chunks.
	wg.Add(1)
	go func() {
		defer wg.Done()
		idx.runBackscan(runCtx, client, tip, tokenAddrs, watchedTopics, tokenByAddr, &swapABI)
	}()

	// Forward polling: catches new blocks as they arrive above tip.
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			return
		case <-ticker.C:
			tipCtx2, tipCancel2 := context.WithTimeout(runCtx, 8*time.Second)
			newTip, err := client.BlockNumber(tipCtx2)
			tipCancel2()
			if err != nil {
				continue
			}

			idx.mu.Lock()
			from := idx.cursor + 1
			idx.mu.Unlock()

			if newTip < from {
				continue
			}

			events := idx.fetchRange(runCtx, client, from, newTip, tokenAddrs, watchedTopics, tokenByAddr)
			for _, ev := range events {
				select {
				case idx.events <- ev:
				default:
					// drop on buffer full
				}
			}

			v4swaps := idx.fetchV4Swaps(runCtx, client, from, newTip, watchedTopics, &swapABI)
			for _, ev := range v4swaps {
				select {
				case idx.v4swaps <- ev:
				default:
					// drop on buffer full
				}
			}

			idx.mu.Lock()
			idx.cursor = newTip
			idx.mu.Unlock()
		}
	}
}

// runBackscan scans blocks backward from startBlock down to 0 in chunks of backChunkSize.
// Each chunk is separated by backChunkInterval to avoid hammering the RPC.
// The store's UNIQUE constraint silently deduplicates any overlap with the forward poller.
func (idx *Indexer) runBackscan(
	ctx context.Context,
	client *ethclient.Client,
	startBlock uint64,
	tokenAddrs []common.Address,
	watchedTopics []common.Hash,
	tokenByAddr map[common.Address]rpc.WatchedToken,
	swapABI *abi.ABI,
) {
	high := startBlock
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backChunkInterval):
		}

		low := uint64(0)
		if high > backChunkSize {
			low = high - backChunkSize
		}

		events := idx.fetchRange(ctx, client, low, high, tokenAddrs, watchedTopics, tokenByAddr)
		for _, ev := range events {
			select {
			case idx.events <- ev:
			default:
			}
		}

		v4swaps := idx.fetchV4Swaps(ctx, client, low, high, watchedTopics, swapABI)
		for _, ev := range v4swaps {
			select {
			case idx.v4swaps <- ev:
			default:
			}
		}

		if low == 0 {
			return // reached genesis
		}
		high = low - 1
	}
}

func (idx *Indexer) fetchRange(
	ctx context.Context,
	client *ethclient.Client,
	from, to uint64,
	tokenAddrs []common.Address,
	watchedTopics []common.Hash,
	tokenByAddr map[common.Address]rpc.WatchedToken,
) []IndexedEvent {
	fromBlock := new(big.Int).SetUint64(from)
	toBlock := new(big.Int).SetUint64(to)

	fCtx, fCancel := context.WithTimeout(ctx, 15*time.Second)
	logsFrom, _ := client.FilterLogs(fCtx, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: tokenAddrs,
		Topics:    [][]common.Hash{{transferSig}, watchedTopics, nil},
	})
	fCancel()

	fCtx2, fCancel2 := context.WithTimeout(ctx, 15*time.Second)
	logsTo, _ := client.FilterLogs(fCtx2, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: tokenAddrs,
		Topics:    [][]common.Hash{{transferSig}, nil, watchedTopics},
	})
	fCancel2()

	// Merge and deduplicate by TxHash+LogIndex.
	seen := make(map[string]struct{})
	var events []IndexedEvent
	for _, l := range append(logsFrom, logsTo...) {
		key := fmt.Sprintf("%s:%d", l.TxHash.Hex(), l.Index)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if ev := decodeTransfer(l, tokenByAddr); ev != nil {
			events = append(events, *ev)
		}
	}
	return events
}

// v4SwapData holds ABI-unpacked non-indexed fields from a V4 Swap event.
type v4SwapData struct {
	Amount0      *big.Int
	Amount1      *big.Int
	SqrtPriceX96 *big.Int
	Liquidity    *big.Int
	Tick         *big.Int
	Fee          *big.Int
}

func (idx *Indexer) fetchV4Swaps(
	ctx context.Context,
	client *ethclient.Client,
	from, to uint64,
	watchedTopics []common.Hash,
	swapABI *abi.ABI,
) []V4SwapEvent {
	poolManager := common.HexToAddress(v4PoolManagerAddress)
	fromBlock := new(big.Int).SetUint64(from)
	toBlock := new(big.Int).SetUint64(to)

	fCtx, fCancel := context.WithTimeout(ctx, 15*time.Second)
	logs, _ := client.FilterLogs(fCtx, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{v4SwapSig}, nil, watchedTopics},
	})
	fCancel()

	var events []V4SwapEvent
	for _, l := range logs {
		if ev := decodeV4Swap(l, swapABI); ev != nil {
			events = append(events, *ev)
		}
	}
	return events
}

func decodeV4Swap(l types.Log, swapABI *abi.ABI) *V4SwapEvent {
	if len(l.Topics) < 3 {
		return nil
	}
	var data v4SwapData
	if err := swapABI.UnpackIntoInterface(&data, "Swap", l.Data); err != nil {
		return nil
	}
	return &V4SwapEvent{
		Block:        l.BlockNumber,
		TxHash:       l.TxHash,
		LogIndex:     uint(l.Index),
		PoolID:       l.Topics[1],
		Sender:       common.BytesToAddress(l.Topics[2].Bytes()[12:]),
		Amount0:      data.Amount0,
		Amount1:      data.Amount1,
		SqrtPriceX96: data.SqrtPriceX96,
		Liquidity:    data.Liquidity,
		Tick:         data.Tick,
		Fee:          data.Fee,
	}
}

func decodeTransfer(l types.Log, tokenByAddr map[common.Address]rpc.WatchedToken) *IndexedEvent {
	if len(l.Topics) < 3 {
		return nil
	}
	t, ok := tokenByAddr[l.Address]
	if !ok {
		return nil
	}
	value := new(big.Int)
	if len(l.Data) >= 32 {
		value.SetBytes(l.Data[:32])
	}
	return &IndexedEvent{
		Block:    l.BlockNumber,
		TxHash:   l.TxHash,
		LogIndex: uint(l.Index),
		From:     common.BytesToAddress(l.Topics[1].Bytes()),
		To:       common.BytesToAddress(l.Topics[2].Bytes()),
		Value:    value,
		Token:    l.Address,
		Symbol:   t.Symbol,
		Decimals: t.Decimals,
	}
}
