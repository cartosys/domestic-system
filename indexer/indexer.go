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
	transferSig      = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	v4InitializeSig  = crypto.Keccak256Hash([]byte("Initialize(bytes32,address,address,uint24,int24,address,uint160,int24)"))
	v4SwapSig        = crypto.Keccak256Hash([]byte("Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)"))
	v4ModifyLiqSig   = crypto.Keccak256Hash([]byte("ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)"))
	v4DonateSig      = crypto.Keccak256Hash([]byte("Donate(bytes32,address,uint256,uint256)"))
	v4TransferSig    = crypto.Keccak256Hash([]byte("Transfer(address,address,address,uint256,uint256)"))
)

// V4DeployBlock is the approximate block at which the V4 PoolManager was deployed on mainnet.
const V4DeployBlock = uint64(21_688_000)

const pollInterval      = 12 * time.Second
const backChunkSize     = uint64(500)
const backChunkInterval = 2 * time.Second
const v4PoolManagerAddress = "0x000000000004444c5dc75cB358380D2e3dE08A90"

// V4EventKind identifies which PoolManager event was emitted.
type V4EventKind uint8

const (
	V4KindInitialize      V4EventKind = iota // Initialize(bytes32,address,address,...) — pool creation
	V4KindSwap                               // Swap(bytes32,address,...)
	V4KindModifyLiquidity                    // ModifyLiquidity(bytes32,address,...)
	V4KindDonate                             // Donate(bytes32,address,...)
	V4KindTransfer                           // Transfer(address,address,address,...) — ERC-6909 claims
)

func (k V4EventKind) String() string {
	switch k {
	case V4KindInitialize:
		return "initialize"
	case V4KindSwap:
		return "swap"
	case V4KindModifyLiquidity:
		return "modify_liquidity"
	case V4KindDonate:
		return "donate"
	case V4KindTransfer:
		return "transfer"
	default:
		return "unknown"
	}
}

// V4PoolEvent represents any Uniswap V4 PoolManager event.
// Fields are populated according to Kind; unused fields are zero/nil.
type V4PoolEvent struct {
	Kind     V4EventKind
	Block    uint64
	TxHash   common.Hash
	LogIndex uint

	// Initialize, Swap, ModifyLiquidity, Donate
	PoolID common.Hash

	// Initialize only: pool key components
	Currency0   common.Address
	Currency1   common.Address
	TickSpacing *big.Int
	Hooks       common.Address

	// Swap, ModifyLiquidity, Donate: direct PoolManager caller (often a router)
	Sender common.Address

	// Swap: signed int128; Donate: unsigned uint256; Initialize: unused
	Amount0 *big.Int
	Amount1 *big.Int

	// Initialize + Swap
	SqrtPriceX96 *big.Int
	Tick         *big.Int

	// Swap only
	Liquidity *big.Int
	Fee       *big.Int

	// ModifyLiquidity only
	TickLower      *big.Int
	TickUpper      *big.Int
	LiquidityDelta *big.Int
	Salt           common.Hash

	// Transfer (ERC-6909) only; Amount reuses Amount0
	From    common.Address
	To      common.Address
	TokenID *big.Int
}

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

// Indexer polls for ERC-20 Transfer events and all Uniswap V4 PoolManager events
// involving saved wallet addresses.
type Indexer struct {
	events     chan IndexedEvent
	poolEvents chan V4PoolEvent
	progress   chan uint64
	cancel     context.CancelFunc
	mu         sync.Mutex
	cursor     uint64
}

// New creates a new Indexer. Call Start to begin indexing.
func New() *Indexer {
	return &Indexer{
		events:     make(chan IndexedEvent, 256),
		poolEvents: make(chan V4PoolEvent, 256),
		progress:   make(chan uint64, 32),
	}
}

// Start begins background polling. Non-blocking.
func (idx *Indexer) Start(rpcURL string, addrs []common.Address, tokens []rpc.WatchedToken) {
	ctx, cancel := context.WithCancel(context.Background())
	idx.cancel = cancel
	go idx.run(ctx, rpcURL, addrs, tokens)
}

// Stop halts the indexer and closes all channels.
func (idx *Indexer) Stop() {
	if idx.cancel != nil {
		idx.cancel()
	}
}

// Events returns the read-only channel of indexed ERC-20 Transfer events.
func (idx *Indexer) Events() <-chan IndexedEvent {
	return idx.events
}

// PoolEvents returns the read-only channel of indexed Uniswap V4 PoolManager events.
func (idx *Indexer) PoolEvents() <-chan V4PoolEvent {
	return idx.poolEvents
}

// Progress returns a read-only channel that emits the current block number each
// time the backward scanner crosses a 10,000-block boundary.
func (idx *Indexer) Progress() <-chan uint64 {
	return idx.progress
}

const v4PoolManagerABI = `[
{
	"anonymous": false,
	"inputs": [
		{"indexed": true,  "name": "id",           "type": "bytes32"},
		{"indexed": true,  "name": "currency0",    "type": "address"},
		{"indexed": true,  "name": "currency1",    "type": "address"},
		{"indexed": false, "name": "fee",          "type": "uint24"},
		{"indexed": false, "name": "tickSpacing",  "type": "int24"},
		{"indexed": false, "name": "hooks",        "type": "address"},
		{"indexed": false, "name": "sqrtPriceX96", "type": "uint160"},
		{"indexed": false, "name": "tick",         "type": "int24"}
	],
	"name": "Initialize",
	"type": "event"
},
{
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
},
{
	"anonymous": false,
	"inputs": [
		{"indexed": true,  "name": "id",            "type": "bytes32"},
		{"indexed": true,  "name": "sender",         "type": "address"},
		{"indexed": false, "name": "tickLower",      "type": "int24"},
		{"indexed": false, "name": "tickUpper",      "type": "int24"},
		{"indexed": false, "name": "liquidityDelta", "type": "int256"},
		{"indexed": false, "name": "salt",           "type": "bytes32"}
	],
	"name": "ModifyLiquidity",
	"type": "event"
},
{
	"anonymous": false,
	"inputs": [
		{"indexed": true,  "name": "id",      "type": "bytes32"},
		{"indexed": true,  "name": "sender",  "type": "address"},
		{"indexed": false, "name": "amount0", "type": "uint256"},
		{"indexed": false, "name": "amount1", "type": "uint256"}
	],
	"name": "Donate",
	"type": "event"
},
{
	"anonymous": false,
	"inputs": [
		{"indexed": false, "name": "caller", "type": "address"},
		{"indexed": true,  "name": "from",   "type": "address"},
		{"indexed": true,  "name": "to",     "type": "address"},
		{"indexed": false, "name": "id",     "type": "uint256"},
		{"indexed": false, "name": "amount", "type": "uint256"}
	],
	"name": "Transfer",
	"type": "event"
}]`

// ABI unpack targets — one per non-indexed data shape.

// v4InitData holds the non-indexed fields of the Initialize event.
// Indexed fields (id, currency0, currency1) are read from Topics directly.
type v4InitData struct {
	Fee          *big.Int
	TickSpacing  *big.Int
	Hooks        common.Address
	SqrtPriceX96 *big.Int
	Tick         *big.Int
}

type v4SwapData struct {
	Amount0      *big.Int
	Amount1      *big.Int
	SqrtPriceX96 *big.Int
	Liquidity    *big.Int
	Tick         *big.Int
	Fee          *big.Int
}

type v4ModifyLiqData struct {
	TickLower      *big.Int
	TickUpper      *big.Int
	LiquidityDelta *big.Int
	Salt           [32]byte
}

type v4DonateData struct {
	Amount0 *big.Int
	Amount1 *big.Int
}

type v4TransferData struct {
	Caller common.Address
	Id     *big.Int
	Amount *big.Int
}

func (idx *Indexer) run(ctx context.Context, rpcURL string, addrs []common.Address, tokens []rpc.WatchedToken) {
	runCtx, runCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		runCancel()
		wg.Wait()
		close(idx.events)
		close(idx.poolEvents)
		close(idx.progress)
	}()

	dialCtx, dialCancel := context.WithTimeout(ctx, 8*time.Second)
	client, err := ethclient.DialContext(dialCtx, rpcURL)
	dialCancel()
	if err != nil {
		return
	}
	defer client.Close()

	pmABI, err := abi.JSON(strings.NewReader(v4PoolManagerABI))
	if err != nil {
		return
	}

	tokenAddrs := make([]common.Address, len(tokens))
	tokenByAddr := make(map[common.Address]rpc.WatchedToken, len(tokens))
	for i, t := range tokens {
		tokenAddrs[i] = t.Address
		tokenByAddr[t.Address] = t
	}

	watchedTopics := make([]common.Hash, len(addrs))
	for i, a := range addrs {
		watchedTopics[i] = common.BytesToHash(a.Bytes())
	}

	tipCtx, tipCancel := context.WithTimeout(ctx, 8*time.Second)
	tip, err := client.BlockNumber(tipCtx)
	tipCancel()
	if err != nil {
		return
	}
	idx.mu.Lock()
	idx.cursor = tip
	idx.mu.Unlock()

	wg.Add(1)
	go func() {
		defer wg.Done()
		idx.runBackscan(runCtx, client, tip, tokenAddrs, watchedTopics, tokenByAddr, &pmABI)
	}()

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

			for _, ev := range idx.fetchRange(runCtx, client, from, newTip, tokenAddrs, watchedTopics, tokenByAddr) {
				select {
				case idx.events <- ev:
				default:
				}
			}

			for _, ev := range idx.fetchV4PoolEvents(runCtx, client, from, newTip, watchedTopics, &pmABI) {
				select {
				case idx.poolEvents <- ev:
				default:
				}
			}

			idx.mu.Lock()
			idx.cursor = newTip
			idx.mu.Unlock()
		}
	}
}

func (idx *Indexer) runBackscan(
	ctx context.Context,
	client *ethclient.Client,
	startBlock uint64,
	tokenAddrs []common.Address,
	watchedTopics []common.Hash,
	tokenByAddr map[common.Address]rpc.WatchedToken,
	pmABI *abi.ABI,
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

		for _, ev := range idx.fetchRange(ctx, client, low, high, tokenAddrs, watchedTopics, tokenByAddr) {
			select {
			case idx.events <- ev:
			default:
			}
		}

		for _, ev := range idx.fetchV4PoolEvents(ctx, client, low, high, watchedTopics, pmABI) {
			select {
			case idx.poolEvents <- ev:
			default:
			}
		}

		if boundary := (high / 10_000) * 10_000; low <= boundary {
			select {
			case idx.progress <- boundary:
			default:
			}
		}

		if low == 0 {
			return
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

// fetchV4PoolEvents queries the PoolManager for all four address-relevant event types.
func (idx *Indexer) fetchV4PoolEvents(
	ctx context.Context,
	client *ethclient.Client,
	from, to uint64,
	watchedTopics []common.Hash,
	pmABI *abi.ABI,
) []V4PoolEvent {
	poolManager := common.HexToAddress(v4PoolManagerAddress)
	fromBlock := new(big.Int).SetUint64(from)
	toBlock := new(big.Int).SetUint64(to)

	// Swap, ModifyLiquidity, Donate: sender is indexed topic[2].
	fCtx, fCancel := context.WithTimeout(ctx, 15*time.Second)
	senderLogs, _ := client.FilterLogs(fCtx, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{v4SwapSig, v4ModifyLiqSig, v4DonateSig}, nil, watchedTopics},
	})
	fCancel()

	// Transfer (ERC-6909): from=topic[1].
	fCtx2, fCancel2 := context.WithTimeout(ctx, 15*time.Second)
	xferFromLogs, _ := client.FilterLogs(fCtx2, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{v4TransferSig}, watchedTopics, nil},
	})
	fCancel2()

	// Transfer (ERC-6909): to=topic[2].
	fCtx3, fCancel3 := context.WithTimeout(ctx, 15*time.Second)
	xferToLogs, _ := client.FilterLogs(fCtx3, ethereum.FilterQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{v4TransferSig}, nil, watchedTopics},
	})
	fCancel3()

	seen := make(map[string]struct{})
	var events []V4PoolEvent
	allLogs := append(senderLogs, append(xferFromLogs, xferToLogs...)...)
	for _, l := range allLogs {
		key := fmt.Sprintf("%s:%d", l.TxHash.Hex(), l.Index)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if ev := decodeV4PoolEvent(l, pmABI); ev != nil {
			events = append(events, *ev)
		}
	}
	return events
}

func decodeV4PoolEvent(l types.Log, pmABI *abi.ABI) *V4PoolEvent {
	if len(l.Topics) < 1 {
		return nil
	}
	base := V4PoolEvent{
		Block:    l.BlockNumber,
		TxHash:   l.TxHash,
		LogIndex: uint(l.Index),
	}
	switch l.Topics[0] {
	case v4InitializeSig:
		// topics: [sig, id(poolId), currency0, currency1]
		if len(l.Topics) < 4 {
			return nil
		}
		var d v4InitData
		if err := pmABI.UnpackIntoInterface(&d, "Initialize", l.Data); err != nil {
			return nil
		}
		base.Kind = V4KindInitialize
		base.PoolID = l.Topics[1]
		base.Currency0 = common.BytesToAddress(l.Topics[2].Bytes()[12:])
		base.Currency1 = common.BytesToAddress(l.Topics[3].Bytes()[12:])
		base.Fee = d.Fee
		base.TickSpacing = d.TickSpacing
		base.Hooks = d.Hooks
		base.SqrtPriceX96 = d.SqrtPriceX96
		base.Tick = d.Tick
		return &base

	case v4SwapSig:
		if len(l.Topics) < 3 {
			return nil
		}
		var d v4SwapData
		if err := pmABI.UnpackIntoInterface(&d, "Swap", l.Data); err != nil {
			return nil
		}
		base.Kind = V4KindSwap
		base.PoolID = l.Topics[1]
		base.Sender = common.BytesToAddress(l.Topics[2].Bytes()[12:])
		base.Amount0 = d.Amount0
		base.Amount1 = d.Amount1
		base.SqrtPriceX96 = d.SqrtPriceX96
		base.Liquidity = d.Liquidity
		base.Tick = d.Tick
		base.Fee = d.Fee
		return &base

	case v4ModifyLiqSig:
		if len(l.Topics) < 3 {
			return nil
		}
		var d v4ModifyLiqData
		if err := pmABI.UnpackIntoInterface(&d, "ModifyLiquidity", l.Data); err != nil {
			return nil
		}
		base.Kind = V4KindModifyLiquidity
		base.PoolID = l.Topics[1]
		base.Sender = common.BytesToAddress(l.Topics[2].Bytes()[12:])
		base.TickLower = d.TickLower
		base.TickUpper = d.TickUpper
		base.LiquidityDelta = d.LiquidityDelta
		base.Salt = common.BytesToHash(d.Salt[:])
		return &base

	case v4DonateSig:
		if len(l.Topics) < 3 {
			return nil
		}
		var d v4DonateData
		if err := pmABI.UnpackIntoInterface(&d, "Donate", l.Data); err != nil {
			return nil
		}
		base.Kind = V4KindDonate
		base.PoolID = l.Topics[1]
		base.Sender = common.BytesToAddress(l.Topics[2].Bytes()[12:])
		base.Amount0 = d.Amount0
		base.Amount1 = d.Amount1
		return &base

	case v4TransferSig:
		if len(l.Topics) < 3 {
			return nil
		}
		var d v4TransferData
		if err := pmABI.UnpackIntoInterface(&d, "Transfer", l.Data); err != nil {
			return nil
		}
		base.Kind = V4KindTransfer
		base.From = common.BytesToAddress(l.Topics[1].Bytes()[12:])
		base.To = common.BytesToAddress(l.Topics[2].Bytes()[12:])
		base.TokenID = d.Id
		base.Amount0 = d.Amount
		return &base
	}
	return nil
}

// FetchAllInitializeEvents returns all pool-creation (Initialize) events from the PoolManager
// in the given block range without any address filter.  For large ranges this may return
// thousands of results; keep ranges narrow when calling interactively.
func FetchAllInitializeEvents(ctx context.Context, client *ethclient.Client, fromBlock, toBlock uint64) ([]V4PoolEvent, error) {
	pmABI, err := abi.JSON(strings.NewReader(v4PoolManagerABI))
	if err != nil {
		return nil, err
	}
	poolManager := common.HexToAddress(v4PoolManagerAddress)
	fCtx, fCancel := context.WithTimeout(ctx, 30*time.Second)
	logs, err := client.FilterLogs(fCtx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{v4InitializeSig}},
	})
	fCancel()
	if err != nil {
		return nil, err
	}
	var events []V4PoolEvent
	for _, l := range logs {
		if ev := decodeV4PoolEvent(l, &pmABI); ev != nil {
			events = append(events, *ev)
		}
	}
	return events, nil
}

// FetchPoolCreation looks up the single Initialize event for poolID between fromBlock and toBlock.
// Returns nil, nil when no matching event is found.
// The pool can only be initialized once, so at most one event will be returned.
func FetchPoolCreation(ctx context.Context, client *ethclient.Client, poolID common.Hash, fromBlock, toBlock uint64) (*V4PoolEvent, error) {
	pmABI, err := abi.JSON(strings.NewReader(v4PoolManagerABI))
	if err != nil {
		return nil, err
	}
	poolManager := common.HexToAddress(v4PoolManagerAddress)
	fCtx, fCancel := context.WithTimeout(ctx, 30*time.Second)
	logs, err := client.FilterLogs(fCtx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{{v4InitializeSig}, {poolID}},
	})
	fCancel()
	if err != nil {
		return nil, err
	}
	for _, l := range logs {
		if ev := decodeV4PoolEvent(l, &pmABI); ev != nil {
			return ev, nil
		}
	}
	return nil, nil
}

// FetchPoolEvents returns all PoolManager events of the given kinds for poolID
// between fromBlock and toBlock. Pass nil kinds to get all event types.
func FetchPoolEvents(ctx context.Context, client *ethclient.Client, poolID common.Hash, fromBlock, toBlock uint64, kinds ...V4EventKind) ([]V4PoolEvent, error) {
	pmABI, err := abi.JSON(strings.NewReader(v4PoolManagerABI))
	if err != nil {
		return nil, err
	}

	wantSigs := make([]common.Hash, 0, len(kinds))
	for _, k := range kinds {
		switch k {
		case V4KindInitialize:
			wantSigs = append(wantSigs, v4InitializeSig)
		case V4KindSwap:
			wantSigs = append(wantSigs, v4SwapSig)
		case V4KindModifyLiquidity:
			wantSigs = append(wantSigs, v4ModifyLiqSig)
		case V4KindDonate:
			wantSigs = append(wantSigs, v4DonateSig)
		}
	}
	if len(wantSigs) == 0 {
		// default: Swap + ModifyLiquidity (the most common post-creation events)
		wantSigs = []common.Hash{v4SwapSig, v4ModifyLiqSig, v4DonateSig}
	}

	poolManager := common.HexToAddress(v4PoolManagerAddress)
	fCtx, fCancel := context.WithTimeout(ctx, 30*time.Second)
	logs, err := client.FilterLogs(fCtx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{wantSigs, {poolID}},
	})
	fCancel()
	if err != nil {
		return nil, err
	}

	var events []V4PoolEvent
	for _, l := range logs {
		if ev := decodeV4PoolEvent(l, &pmABI); ev != nil {
			events = append(events, *ev)
		}
	}
	return events, nil
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
