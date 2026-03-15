package helpers

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// V4PoolManagerAddress is the canonical Uniswap V4 PoolManager address on Ethereum mainnet.
const V4PoolManagerAddress = "0x000000000004444c5dc75cB358380D2e3dE08A90"

// V4StateViewAddress is the Uniswap V4 StateView peripheral contract on Ethereum mainnet.
// getSlot0 and getLiquidity must be called here (not on PoolManager) because PoolManager
// uses custom packed storage (extsload) that is not readable via a standard eth_call.
const V4StateViewAddress = "0x7fFE42C4a5DEeA5b0feC41C94c136Cf115597227"

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

// ---- Event types ----

type v4PoolKey struct {
	Currency0   common.Address
	Currency1   common.Address
	Fee         uint32
	TickSpacing int32
	Hooks       common.Address
}

type v4InitializeEvent struct {
	Id           common.Hash
	Currency0    common.Address
	Currency1    common.Address
	Fee          *big.Int
	TickSpacing  *big.Int
	Hooks        common.Address
	SqrtPriceX96 *big.Int
	Tick         *big.Int
}

type v4ModifyLiquidityEvent struct {
	Id             common.Hash
	Sender         common.Address
	TickLower      *big.Int
	TickUpper      *big.Int
	LiquidityDelta *big.Int
	Salt           [32]byte
}

type v4SwapEvent struct {
	Id           common.Hash
	Sender       common.Address
	Amount0      *big.Int
	Amount1      *big.Int
	SqrtPriceX96 *big.Int
	Liquidity    *big.Int
	Tick         *big.Int
	Fee          *big.Int
}

type v4DonateEvent struct {
	Id      common.Hash
	Sender  common.Address
	Amount0 *big.Int
	Amount1 *big.Int
}

type v4OperatorSetEvent struct {
	Owner    common.Address
	Operator common.Address
	Approved bool
}

type v4TransferEvent struct {
	From   common.Address
	To     common.Address
	Id     *big.Int
	Caller common.Address
	Amount *big.Int
}

type v4ProtocolFeeUpdatedEvent struct {
	Id          common.Hash
	ProtocolFee *big.Int
}

// ---- Helpers ----

func v4ShortAddr(a common.Address) string {
	s := a.Hex()
	if len(s) <= 10 {
		return s
	}
	return s[:6] + "…" + s[len(s)-4:]
}

func v4ShortHash(h common.Hash) string {
	s := h.Hex()
	return s[:10] + "…" + s[len(s)-6:]
}

// v4FadePoolID renders a shortened pool ID with the domestic-system title gradient
// and wraps it in an OSC 8 hyperlink using the poolinfo:// scheme so the TUI can
// intercept clicks and show the Pool Info popup.
func v4FadePoolID(h common.Hash) string {
	display := FadeString(v4ShortHash(h), "#7EE787", "#82CFFD")
	return ansi.SetHyperlink("poolinfo://"+h.Hex()) + display + ansi.ResetHyperlink()
}

// v4HyperAddr returns a FadeString-coloured, OSC 8 hyperlinked short address
// pointing to the Etherscan address page.
func v4HyperAddr(a common.Address) string {
	display := FadeString(v4ShortAddr(a), "#F25D94", "#EDFF82")
	return ansi.SetHyperlink("https://etherscan.io/address/"+a.Hex()) + display + ansi.ResetHyperlink()
}

// v4HyperTxHash returns a FadeString-coloured, OSC 8 hyperlinked short hash
// pointing to the Etherscan transaction page.
func v4HyperTxHash(h common.Hash) string {
	display := FadeString(v4ShortHash(h), "#F25D94", "#EDFF82")
	return ansi.SetHyperlink("https://etherscan.io/tx/"+h.Hex()) + display + ansi.ResetHyperlink()
}

func v4SignedStr(x *big.Int) string {
	if x == nil {
		return "nil"
	}
	return x.String()
}

// v4CurrencyLabel returns "NATIVE" for the zero address, or a FadeString-coloured
// OSC 8 hyperlinked full address for any real ERC-20 token.
func v4CurrencyLabel(a common.Address) string {
	if (a == common.Address{}) {
		return "NATIVE"
	}
	display := FadeString(a.Hex(), "#F25D94", "#EDFF82")
	return ansi.SetHyperlink("https://etherscan.io/address/"+a.Hex()) + display + ansi.ResetHyperlink()
}

func v4ResolvePool(mu *sync.RWMutex, poolKeys map[common.Hash]v4PoolKey, id common.Hash) (pair, hooks string) {
	mu.RLock()
	key, ok := poolKeys[id]
	mu.RUnlock()
	if !ok {
		return "??/??", "??"
	}
	pair = fmt.Sprintf("%s/%s", v4CurrencyLabel(key.Currency0), v4CurrencyLabel(key.Currency1))
	return pair, v4HyperAddr(key.Hooks)
}

// ---- Per-event formatters (return line string) ----

func v4FmtInitialize(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]v4PoolKey) (string, error) {
	if len(lg.Topics) < 4 {
		return "", fmt.Errorf("Initialize: want 4 topics, got %d", len(lg.Topics))
	}
	var ev v4InitializeEvent
	ev.Id = lg.Topics[1]
	ev.Currency0 = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	ev.Currency1 = common.BytesToAddress(lg.Topics[3].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "Initialize", lg.Data); err != nil {
		return "", fmt.Errorf("unpack Initialize: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	mu.Lock()
	poolKeys[ev.Id] = v4PoolKey{
		Currency0:   ev.Currency0,
		Currency1:   ev.Currency1,
		Fee:         uint32(ev.Fee.Uint64()),
		TickSpacing: int32(ev.TickSpacing.Int64()),
		Hooks:       ev.Hooks,
	}
	mu.Unlock()
	return fmt.Sprintf(
		"[Initialize]         block=%d tx=%s poolId=%s c0=%s c1=%s fee=%s tickSpacing=%s hooks=%s sqrtPrice=%s tick=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4FadePoolID(ev.Id),
		v4CurrencyLabel(ev.Currency0), v4CurrencyLabel(ev.Currency1),
		ev.Fee.String(), ev.TickSpacing.String(), v4HyperAddr(ev.Hooks),
		ev.SqrtPriceX96.String(), ev.Tick.String(),
	), nil
}

func v4FmtModifyLiquidity(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]v4PoolKey) (string, error) {
	if len(lg.Topics) < 3 {
		return "", fmt.Errorf("ModifyLiquidity: want 3 topics, got %d", len(lg.Topics))
	}
	var ev v4ModifyLiquidityEvent
	ev.Id = lg.Topics[1]
	ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "ModifyLiquidity", lg.Data); err != nil {
		return "", fmt.Errorf("unpack ModifyLiquidity: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	pair, hooks := v4ResolvePool(mu, poolKeys, ev.Id)
	action := "add"
	if ev.LiquidityDelta != nil && ev.LiquidityDelta.Sign() < 0 {
		action = "remove"
	}
	return fmt.Sprintf(
		"[ModifyLiquidity]    block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s action=%s delta=%s tickLow=%s tickHigh=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4FadePoolID(ev.Id),
		pair, hooks, v4HyperAddr(ev.Sender),
		action, v4SignedStr(ev.LiquidityDelta),
		v4SignedStr(ev.TickLower), v4SignedStr(ev.TickUpper),
	), nil
}

func v4FmtSwap(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]v4PoolKey) (string, error) {
	if len(lg.Topics) < 3 {
		return "", fmt.Errorf("Swap: want 3 topics, got %d", len(lg.Topics))
	}
	var ev v4SwapEvent
	ev.Id = lg.Topics[1]
	ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "Swap", lg.Data); err != nil {
		return "", fmt.Errorf("unpack Swap: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	pair, hooks := v4ResolvePool(mu, poolKeys, ev.Id)
	dirHint := ""
	if ev.Amount0 != nil && ev.Amount0.Sign() < 0 {
		dirHint = " (token0 out)"
	} else if ev.Amount1 != nil && ev.Amount1.Sign() < 0 {
		dirHint = " (token1 out)"
	}
	return fmt.Sprintf(
		"[Swap]               block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s amt0=%s amt1=%s tick=%s fee=%s%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4FadePoolID(ev.Id),
		pair, hooks, v4HyperAddr(ev.Sender),
		v4SignedStr(ev.Amount0), v4SignedStr(ev.Amount1),
		v4SignedStr(ev.Tick), ev.Fee.String(), dirHint,
	), nil
}

func v4FmtDonate(parsedABI *abi.ABI, lg types.Log, mu *sync.RWMutex, poolKeys map[common.Hash]v4PoolKey) (string, error) {
	if len(lg.Topics) < 3 {
		return "", fmt.Errorf("Donate: want 3 topics, got %d", len(lg.Topics))
	}
	var ev v4DonateEvent
	ev.Id = lg.Topics[1]
	ev.Sender = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "Donate", lg.Data); err != nil {
		return "", fmt.Errorf("unpack Donate: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	pair, hooks := v4ResolvePool(mu, poolKeys, ev.Id)
	return fmt.Sprintf(
		"[Donate]             block=%d tx=%s poolId=%s pair=%s hooks=%s sender=%s amt0=%s amt1=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4FadePoolID(ev.Id),
		pair, hooks, v4HyperAddr(ev.Sender),
		ev.Amount0.String(), ev.Amount1.String(),
	), nil
}

func v4FmtOperatorSet(parsedABI *abi.ABI, lg types.Log) (string, error) {
	if len(lg.Topics) < 3 {
		return "", fmt.Errorf("OperatorSet: want 3 topics, got %d", len(lg.Topics))
	}
	var ev v4OperatorSetEvent
	ev.Owner = common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	ev.Operator = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	if err := parsedABI.UnpackIntoInterface(&ev, "OperatorSet", lg.Data); err != nil {
		return "", fmt.Errorf("unpack OperatorSet: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	return fmt.Sprintf(
		"[OperatorSet]        block=%d tx=%s owner=%s operator=%s approved=%v",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash),
		v4HyperAddr(ev.Owner), v4HyperAddr(ev.Operator), ev.Approved,
	), nil
}

func v4FmtTransfer(parsedABI *abi.ABI, lg types.Log) (string, error) {
	if len(lg.Topics) < 4 {
		return "", fmt.Errorf("Transfer: want 4 topics, got %d", len(lg.Topics))
	}
	var ev v4TransferEvent
	ev.From = common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	ev.To = common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	ev.Id = new(big.Int).SetBytes(lg.Topics[3].Bytes())
	if err := parsedABI.UnpackIntoInterface(&ev, "Transfer", lg.Data); err != nil {
		return "", fmt.Errorf("unpack Transfer: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	return fmt.Sprintf(
		"[Transfer]           block=%d tx=%s from=%s to=%s tokenId=%s amount=%s caller=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash),
		v4HyperAddr(ev.From), v4HyperAddr(ev.To),
		ev.Id.String(), ev.Amount.String(), v4HyperAddr(ev.Caller),
	), nil
}

func v4FmtProtocolFeeUpdated(parsedABI *abi.ABI, lg types.Log) (string, error) {
	if len(lg.Topics) < 2 {
		return "", fmt.Errorf("ProtocolFeeUpdated: want 2 topics, got %d", len(lg.Topics))
	}
	var ev v4ProtocolFeeUpdatedEvent
	ev.Id = lg.Topics[1]
	if err := parsedABI.UnpackIntoInterface(&ev, "ProtocolFeeUpdated", lg.Data); err != nil {
		return "", fmt.Errorf("unpack ProtocolFeeUpdated: %w (data=%s)", err, hex.EncodeToString(lg.Data))
	}
	return fmt.Sprintf(
		"[ProtocolFeeUpdated] block=%d tx=%s poolId=%s protocolFee=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4FadePoolID(ev.Id), ev.ProtocolFee.String(),
	), nil
}

func v4FmtProtocolFeeControllerUpdated(lg types.Log) (string, error) {
	if len(lg.Topics) < 2 {
		return "", fmt.Errorf("ProtocolFeeControllerUpdated: want 2 topics, got %d", len(lg.Topics))
	}
	controller := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	return fmt.Sprintf(
		"[FeeControllerUpd]   block=%d tx=%s newController=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4HyperAddr(controller),
	), nil
}

func v4FmtOwnershipTransferred(lg types.Log) (string, error) {
	if len(lg.Topics) < 3 {
		return "", fmt.Errorf("OwnershipTransferred: want 3 topics, got %d", len(lg.Topics))
	}
	prev := common.BytesToAddress(lg.Topics[1].Bytes()[12:])
	next := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
	return fmt.Sprintf(
		"[OwnershipXfer]      block=%d tx=%s from=%s to=%s",
		lg.BlockNumber, v4HyperTxHash(lg.TxHash), v4HyperAddr(prev), v4HyperAddr(next),
	), nil
}

// v4FormatLog dispatches a log entry to the appropriate formatter and returns the line.
func v4FormatLog(
	parsedABI *abi.ABI,
	lg types.Log,
	eventNames map[common.Hash]string,
	mu *sync.RWMutex,
	poolKeys map[common.Hash]v4PoolKey,
) (string, error) {
	if len(lg.Topics) == 0 {
		return "", nil
	}
	name, ok := eventNames[lg.Topics[0]]
	if !ok {
		return "", nil
	}
	switch name {
	case "Initialize":
		return v4FmtInitialize(parsedABI, lg, mu, poolKeys)
	case "ModifyLiquidity":
		return v4FmtModifyLiquidity(parsedABI, lg, mu, poolKeys)
	case "Swap":
		return v4FmtSwap(parsedABI, lg, mu, poolKeys)
	case "Donate":
		return v4FmtDonate(parsedABI, lg, mu, poolKeys)
	case "OperatorSet":
		return v4FmtOperatorSet(parsedABI, lg)
	case "Transfer":
		return v4FmtTransfer(parsedABI, lg)
	case "ProtocolFeeUpdated":
		return v4FmtProtocolFeeUpdated(parsedABI, lg)
	case "ProtocolFeeControllerUpdated":
		return v4FmtProtocolFeeControllerUpdated(lg)
	case "OwnershipTransferred":
		return v4FmtOwnershipTransferred(lg)
	}
	return "", nil
}

// ---- PoolEventMonitor ----

// PoolEventMonitor subscribes to Uniswap V4 PoolManager events and streams
// formatted event lines through a channel for display in the TUI log panel.
type PoolEventMonitor struct {
	lines  chan string
	cancel context.CancelFunc
}

// NewPoolEventMonitor creates a new PoolEventMonitor ready to be started.
func NewPoolEventMonitor() *PoolEventMonitor {
	return &PoolEventMonitor{
		lines: make(chan string, 512),
	}
}

// Start dials the given WebSocket RPC URL and begins streaming pool events.
// The WebSocket URL must use the wss:// or ws:// scheme for live subscriptions.
// Events are available via Lines(). The goroutine exits when Stop() is called
// or a fatal error occurs, after which the Lines() channel is closed.
func (m *PoolEventMonitor) Start(wsURL string) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.run(ctx, wsURL)
}

// Stop cancels the monitor's context, causing the background goroutine to exit.
func (m *PoolEventMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Lines returns the read-only channel of formatted event line strings.
// The channel is closed when the monitor stops.
func (m *PoolEventMonitor) Lines() <-chan string {
	return m.lines
}

func (m *PoolEventMonitor) emit(line string) {
	select {
	case m.lines <- line:
	default:
		// Drop if buffer full to avoid blocking the subscription loop.
	}
}

func (m *PoolEventMonitor) run(ctx context.Context, wsURL string) {
	defer close(m.lines)

	if wsURL == "" {
		m.emit("[PoolMonitor] ERROR: no RPC URL configured")
		return
	}
	// Convert HTTP scheme to WebSocket scheme.
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + wsURL[len("https://"):]
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + wsURL[len("http://"):]
	}
	// Replace port 8545 with 8546 (standard WebSocket port for Ethereum nodes).
	wsURL = strings.ReplaceAll(wsURL, ":8545", ":8546")
	if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
		m.emit(fmt.Sprintf("[PoolMonitor] ERROR: WebSocket URL required (got %q). Set a wss:// RPC endpoint.", wsURL))
		return
	}

	parsedABI, err := abi.JSON(strings.NewReader(poolManagerEventsABI))
	if err != nil {
		m.emit(fmt.Sprintf("[PoolMonitor] ERROR: parse ABI: %v", err))
		return
	}

	eventNames := make(map[common.Hash]string, len(parsedABI.Events))
	allSigs := make([]common.Hash, 0, len(parsedABI.Events))
	for name, ev := range parsedABI.Events {
		eventNames[ev.ID] = name
		allSigs = append(allSigs, ev.ID)
	}

	client, err := ethclient.DialContext(ctx, wsURL)
	if err != nil {
		m.emit(fmt.Sprintf("[PoolMonitor] ERROR: dial %s: %v", wsURL, err))
		return
	}
	defer client.Close()

	poolManager := common.HexToAddress(V4PoolManagerAddress)

	var (
		mu       sync.RWMutex
		poolKeys = make(map[common.Hash]v4PoolKey)
	)

	query := ethereum.FilterQuery{
		Addresses: []common.Address{poolManager},
		Topics:    [][]common.Hash{allSigs},
	}

	logCh := make(chan types.Log, 1024)
	sub, err := client.SubscribeFilterLogs(ctx, query, logCh)
	if err != nil {
		m.emit(fmt.Sprintf("[PoolMonitor] ERROR: subscribe: %v", err))
		return
	}

	m.emit(fmt.Sprintf("[PoolMonitor] Listening… PoolManager=%s", poolManager.Hex()))

	for {
		select {
		case <-ctx.Done():
			return

		case subErr, ok := <-sub.Err():
			if !ok {
				return
			}
			m.emit(fmt.Sprintf("[PoolMonitor] ERROR: subscription: %v", subErr))
			return

		case lg := <-logCh:
			line, fmtErr := v4FormatLog(&parsedABI, lg, eventNames, &mu, poolKeys)
			if fmtErr != nil {
				m.emit(fmt.Sprintf("[PoolMonitor] decode error: %v", fmtErr))
			} else if line != "" {
				m.emit(line)
			}
		}
	}
}

// ---- Pool Info (on-demand contract reads) ----

const poolManagerViewABI = `[
  {
    "inputs": [{"internalType": "PoolId", "name": "id", "type": "bytes32"}],
    "name": "getSlot0",
    "outputs": [
      {"internalType": "uint160", "name": "sqrtPriceX96", "type": "uint160"},
      {"internalType": "int24",   "name": "tick",          "type": "int24"},
      {"internalType": "uint24",  "name": "protocolFee",   "type": "uint24"},
      {"internalType": "uint24",  "name": "lpFee",         "type": "uint24"}
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [{"internalType": "PoolId", "name": "id", "type": "bytes32"}],
    "name": "getLiquidity",
    "outputs": [{"internalType": "uint128", "name": "liquidity", "type": "uint128"}],
    "stateMutability": "view",
    "type": "function"
  }
]`

// PoolInfo holds the live on-chain state for a single Uniswap V4 pool.
type PoolInfo struct {
	// Pool key (from Initialize event)
	Currency0   string `json:"currency0,omitempty"`
	Currency1   string `json:"currency1,omitempty"`
	Fee         uint32 `json:"fee,omitempty"`
	TickSpacing int32  `json:"tickSpacing,omitempty"`
	Hooks       string `json:"hooks,omitempty"`
	// Live state (from StateView)
	SqrtPriceX96 string `json:"sqrtPriceX96"`
	Tick         int64  `json:"tick"`
	ProtocolFee  uint32 `json:"protocolFee"`
	LpFee        uint32 `json:"lpFee"`
	Liquidity    string `json:"liquidity"`
}

// v4PoolManagerDeployBlock is the approximate mainnet block at which the V4
// PoolManager was deployed, used as the lower bound for Initialize event lookups.
const v4PoolManagerDeployBlock = 21688000

// poolCurrencyStr returns "NATIVE" for the zero address, or the checksummed hex address.
func poolCurrencyStr(a common.Address) string {
	if (a == common.Address{}) {
		return "NATIVE"
	}
	return a.Hex()
}

// FetchPoolInfo calls getSlot0 and getLiquidity on the V4 StateView for the
// given pool ID, and looks up the pool key from the Initialize event log.
func FetchPoolInfo(rpcURL string, poolID common.Hash) (*PoolInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	defer client.Close()

	parsedABI, err := abi.JSON(strings.NewReader(poolManagerViewABI))
	if err != nil {
		return nil, fmt.Errorf("parse ABI: %w", err)
	}

	stateView := common.HexToAddress(V4StateViewAddress)

	// getSlot0
	slot0Calldata, err := parsedABI.Pack("getSlot0", poolID)
	if err != nil {
		return nil, fmt.Errorf("pack getSlot0: %w", err)
	}
	slot0Raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &stateView, Data: slot0Calldata}, nil)
	if err != nil {
		return nil, fmt.Errorf("call getSlot0: %w", err)
	}
	slot0Vals, err := parsedABI.Unpack("getSlot0", slot0Raw)
	if err != nil {
		return nil, fmt.Errorf("unpack getSlot0: %w (raw=%s)", err, hex.EncodeToString(slot0Raw))
	}

	// getLiquidity
	liqCalldata, err := parsedABI.Pack("getLiquidity", poolID)
	if err != nil {
		return nil, fmt.Errorf("pack getLiquidity: %w", err)
	}
	liqRaw, err := client.CallContract(ctx, ethereum.CallMsg{To: &stateView, Data: liqCalldata}, nil)
	if err != nil {
		return nil, fmt.Errorf("call getLiquidity: %w", err)
	}
	liqVals, err := parsedABI.Unpack("getLiquidity", liqRaw)
	if err != nil {
		return nil, fmt.Errorf("unpack getLiquidity: %w", err)
	}

	sqrtPriceX96 := slot0Vals[0].(*big.Int)
	tick := slot0Vals[1].(*big.Int).Int64()
	protocolFee := uint32(slot0Vals[2].(*big.Int).Uint64())
	lpFee := uint32(slot0Vals[3].(*big.Int).Uint64())
	liquidity := liqVals[0].(*big.Int)

	info := &PoolInfo{
		SqrtPriceX96: sqrtPriceX96.String(),
		Tick:         tick,
		ProtocolFee:  protocolFee,
		LpFee:        lpFee,
		Liquidity:    liquidity.String(),
	}

	// Look up pool key from the Initialize event log.
	// The Initialize event has poolId as topics[1], currency0 as topics[2],
	// currency1 as topics[3], and fee/tickSpacing/hooks in non-indexed data.
	eventsABI, err := abi.JSON(strings.NewReader(poolManagerEventsABI))
	if err == nil {
		poolManager := common.HexToAddress(V4PoolManagerAddress)
		q := ethereum.FilterQuery{
			FromBlock: big.NewInt(v4PoolManagerDeployBlock),
			Addresses: []common.Address{poolManager},
			Topics:    [][]common.Hash{{eventsABI.Events["Initialize"].ID}, {poolID}},
		}
		if logs, err := client.FilterLogs(ctx, q); err == nil && len(logs) > 0 {
			lg := logs[0]
			if len(lg.Topics) >= 4 {
				currency0 := common.BytesToAddress(lg.Topics[2].Bytes()[12:])
				currency1 := common.BytesToAddress(lg.Topics[3].Bytes()[12:])
				var ev v4InitializeEvent
				if err := eventsABI.UnpackIntoInterface(&ev, "Initialize", lg.Data); err == nil {
					info.Currency0 = poolCurrencyStr(currency0)
					info.Currency1 = poolCurrencyStr(currency1)
					info.Fee = uint32(ev.Fee.Uint64())
					info.TickSpacing = int32(ev.TickSpacing.Int64())
					info.Hooks = ev.Hooks.Hex()
				}
			}
		}
	}

	return info, nil
}
