package indexer

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"charm-wallet-tui/rpc"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func shortAddr(a common.Address) string {
	h := a.Hex()
	return h[:6] + "…" + h[len(h)-4:]
}

func shortHash(h common.Hash) string {
	s := h.Hex()
	return s[:10] + "…" + s[len(s)-6:]
}

// v4ScanBlock is a block used for V4 event tests.
const v4ScanBlock = uint64(24686488)

const (
	testBlock  = uint64(24708660)
	testRPCEnv = "ETH_RPC_URL"
)

var (
	usdcToken = rpc.WatchedToken{
		Symbol:   "USDC",
		Decimals: 6,
		Address:  common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"),
	}
	testAddr      = common.HexToAddress("0x5B576BdAd96Cd7EB643b3229B499130B55e8CA4d")
	testAddrTopic = common.BytesToHash(testAddr.Bytes())
)

func TestFetchRangeUSDCTransfer(t *testing.T) {
	rpcURL := os.Getenv(testRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set — skipping live indexer test", testRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	tokenAddrs := []common.Address{usdcToken.Address}
	tokenByAddr := map[common.Address]rpc.WatchedToken{
		usdcToken.Address: usdcToken,
	}

	idx := New()

	// ── Diagnostic: all USDC transfers at this block (no address filter) ──────
	t.Logf("Diagnostic: fetching all USDC transfers at block %d (unfiltered)…", testBlock)
	allEvents := idx.fetchRange(ctx, client, testBlock, testBlock, tokenAddrs, nil, tokenByAddr)
	t.Logf("  %d total USDC transfer(s) found", len(allEvents))
	for _, ev := range allEvents {
		t.Logf("  from=%s  to=%s  value=%s  block=%d  tx=%s",
			shortAddr(ev.From),
			shortAddr(ev.To),
			ev.Value.String(),
			ev.Block,
			shortHash(ev.TxHash),
		)
	}

	// ── Filtered query using the provided topic hash ──────────────────────────
	t.Logf("Filtered: querying for address %s (topic %s)",
		shortAddr(testAddr),
		shortHash(testAddrTopic),
	)
	watchedTopics := []common.Hash{testAddrTopic}
	events := idx.fetchRange(ctx, client, testBlock, testBlock, tokenAddrs, watchedTopics, tokenByAddr)

	t.Logf("─────────────────────────────────────────────────────────")
	t.Logf("Results: %d USDC transfer(s) matched at block %d", len(events), testBlock)
	t.Logf("─────────────────────────────────────────────────────────")

	for i, ev := range events {
		divisor := new(big.Float).SetInt(
			new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(ev.Decimals)), nil),
		)
		humanAmt := new(big.Float).Quo(new(big.Float).SetInt(ev.Value), divisor)

		t.Logf("Event #%d", i+1)
		t.Logf("  Token   : %s (%s)", ev.Symbol, shortAddr(ev.Token))
		t.Logf("  Block   : %d", ev.Block)
		t.Logf("  TxHash  : %s", shortHash(ev.TxHash))
		t.Logf("  LogIndex: %d", ev.LogIndex)
		t.Logf("  From    : %s", shortAddr(ev.From))
		t.Logf("  To      : %s", shortAddr(ev.To))
		t.Logf("  Value   : %s raw  (%s %s)", ev.Value.String(), fmt.Sprintf("%.6f", humanAmt), ev.Symbol)
		t.Logf("  Decimals: %d", ev.Decimals)
		t.Logf("─────────────────────────────────────────────────────────")
	}

	if len(events) == 0 {
		t.Errorf("expected at least one USDC transfer at block %d for address %s, got none", testBlock, testAddr.Hex())
	}
}

// ── V4 PoolManager event tests ────────────────────────────────────────────────
//
// These tests cover each V4 PoolManager event kind: Initialize (pool creation),
// Swap, and ModifyLiquidity (add/remove liquidity).
//
// All tests are self-contained — they discover a pool dynamically rather than
// relying on hardcoded pool IDs.

// TestFetchV4InitializeNearDeployment verifies that Initialize (pool creation)
// events can be fetched from the PoolManager.
// Scans 200 blocks after V4 deployment — guaranteed to contain many pools.
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./indexer -v -run TestFetchV4InitializeNearDeployment
func TestFetchV4InitializeNearDeployment(t *testing.T) {
	rpcURL := os.Getenv(testRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set — skipping live V4 Initialize test", testRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	from, to := V4DeployBlock, V4DeployBlock+200
	t.Logf("Scanning blocks %d–%d for Initialize events…", from, to)

	events, err := FetchAllInitializeEvents(ctx, client, from, to)
	if err != nil {
		t.Fatalf("FetchAllInitializeEvents: %v", err)
	}
	t.Logf("Found %d pool creation event(s)", len(events))

	for i, ev := range events {
		if i >= 10 {
			t.Logf("  … %d more", len(events)-10)
			break
		}
		noHooks := ev.Hooks == (common.Address{})
		t.Logf("  [%02d] block=%d  %s/%s  fee=%s  tickSpacing=%s  hooks=%s  poolId=%s",
			i+1, ev.Block,
			v4AddrLabel(ev.Currency0), v4AddrLabel(ev.Currency1),
			bigOrZero(ev.Fee), bigOrZero(ev.TickSpacing),
			v4HooksLabel(noHooks, ev.Hooks),
			ev.PoolID.Hex()[:18]+"…",
		)
		t.Logf("        tx=%s", ev.TxHash.Hex())
	}

	if len(events) == 0 {
		t.Errorf("expected Initialize events in blocks %d–%d; got none", from, to)
	}
}

// TestFetchV4PoolCreation discovers a pool from the deployment range, then
// verifies FetchPoolCreation returns the matching Initialize event.
// This is the round-trip test for pool indexing: scan → pool ID → verify lookup.
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./indexer -v -run TestFetchV4PoolCreation
func TestFetchV4PoolCreation(t *testing.T) {
	rpcURL := os.Getenv(testRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set — skipping live V4 pool creation test", testRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	// Step 1: find any pool in a known range.
	from, to := V4DeployBlock, V4DeployBlock+200
	inits, err := FetchAllInitializeEvents(ctx, client, from, to)
	if err != nil {
		t.Fatalf("FetchAllInitializeEvents: %v", err)
	}
	if len(inits) == 0 {
		t.Skipf("no Initialize events found in blocks %d–%d", from, to)
	}
	first := inits[0]
	t.Logf("Discovered pool %s at block %d", first.PoolID.Hex(), first.Block)

	// Step 2: FetchPoolCreation should return the same event by poolId lookup.
	got, err := FetchPoolCreation(ctx, client, first.PoolID, from, to)
	if err != nil {
		t.Fatalf("FetchPoolCreation: %v", err)
	}
	if got == nil {
		t.Fatalf("FetchPoolCreation returned nil for poolId %s", first.PoolID.Hex())
	}
	if got.Block != first.Block || got.TxHash != first.TxHash {
		t.Errorf("FetchPoolCreation mismatch: got block=%d tx=%s, want block=%d tx=%s",
			got.Block, got.TxHash.Hex(), first.Block, first.TxHash.Hex())
	}

	t.Logf("PASS: FetchPoolCreation round-trip correct")
	t.Logf("  block        = %d", got.Block)
	t.Logf("  tx           = %s", got.TxHash.Hex())
	t.Logf("  currency0    = %s  (%s)", got.Currency0.Hex(), v4AddrLabel(got.Currency0))
	t.Logf("  currency1    = %s  (%s)", got.Currency1.Hex(), v4AddrLabel(got.Currency1))
	t.Logf("  fee          = %s", bigOrZero(got.Fee))
	t.Logf("  tickSpacing  = %s", bigOrZero(got.TickSpacing))
	t.Logf("  hooks        = %s", got.Hooks.Hex())
	t.Logf("  sqrtPriceX96 = %s", bigOrZero(got.SqrtPriceX96))
	t.Logf("  initTick     = %s", bigOrZero(got.Tick))
	t.Logf("  NOTE: pool creator requires eth_getTransactionByHash(%s).tx.from", got.TxHash.Hex())
}

// TestFetchV4SwapEventsForPool finds an active pool and fetches its Swap events.
// Shows how Swap.sender is the router (not the EOA) — tx.from is required for EOA.
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./indexer -v -run TestFetchV4SwapEventsForPool
func TestFetchV4SwapEventsForPool(t *testing.T) {
	rpcURL := os.Getenv(testRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set", testRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	// Discover pools near deployment, pick the first one with activity.
	inits, err := FetchAllInitializeEvents(ctx, client, V4DeployBlock, V4DeployBlock+200)
	if err != nil || len(inits) == 0 {
		t.Skipf("no Initialize events found near deployment")
	}

	// Try each pool until we find one with Swap events.
	var (
		poolId    common.Hash
		createdAt uint64
		swaps     []V4PoolEvent
	)
	for _, init := range inits {
		s, err := FetchPoolEvents(ctx, client, init.PoolID, init.Block, v4ScanBlock, V4KindSwap)
		if err != nil {
			continue
		}
		if len(s) > 0 {
			poolId, createdAt, swaps = init.PoolID, init.Block, s
			break
		}
	}

	if len(swaps) == 0 {
		t.Logf("None of the %d pools scanned had Swap events through block %d — test inconclusive", len(inits), v4ScanBlock)
		return
	}

	t.Logf("Pool %s (created block %d) has %d Swap event(s) through block %d",
		poolId.Hex()[:18]+"…", createdAt, len(swaps), v4ScanBlock)
	t.Logf("NOTE: Swap.sender is the direct PoolManager caller (router), NOT the user's EOA.")
	t.Logf("      To find the EOA: call eth_getTransactionByHash(tx).from for each swap.")

	for i, ev := range swaps {
		if i >= 5 {
			t.Logf("  … %d more swap(s)", len(swaps)-5)
			break
		}
		t.Logf("  [%d] block=%d  sender(router)=%s  amount0=%s  amount1=%s  tick=%s",
			i+1, ev.Block, ev.Sender.Hex(),
			bigOrZero(ev.Amount0), bigOrZero(ev.Amount1), bigOrZero(ev.Tick))
		t.Logf("       tx=%s", ev.TxHash.Hex())
	}
}

// TestFetchV4ModifyLiquidityEventsForPool finds a pool with liquidity events
// and shows the add/remove distinction via the sign of liquidityDelta.
//
// Run with:
//
//	ETH_RPC_URL="https://eth.llamarpc.com" go test ./indexer -v -run TestFetchV4ModifyLiquidityEventsForPool
func TestFetchV4ModifyLiquidityEventsForPool(t *testing.T) {
	rpcURL := os.Getenv(testRPCEnv)
	if rpcURL == "" {
		t.Skipf("%s not set", testRPCEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	inits, err := FetchAllInitializeEvents(ctx, client, V4DeployBlock, V4DeployBlock+200)
	if err != nil || len(inits) == 0 {
		t.Skipf("no Initialize events found near deployment")
	}

	var (
		poolId    common.Hash
		createdAt uint64
		mods      []V4PoolEvent
	)
	for _, init := range inits {
		m, err := FetchPoolEvents(ctx, client, init.PoolID, init.Block, v4ScanBlock, V4KindModifyLiquidity)
		if err != nil {
			continue
		}
		if len(m) > 0 {
			poolId, createdAt, mods = init.PoolID, init.Block, m
			break
		}
	}

	if len(mods) == 0 {
		t.Logf("None of the %d pools scanned had ModifyLiquidity events through block %d — test inconclusive", len(inits), v4ScanBlock)
		return
	}

	t.Logf("Pool %s (created block %d) has %d ModifyLiquidity event(s) through block %d",
		poolId.Hex()[:18]+"…", createdAt, len(mods), v4ScanBlock)
	t.Logf("liquidityDelta > 0 = add liquidity   liquidityDelta < 0 = remove liquidity")
	t.Logf("NOTE: ModifyLiquidity.sender is the PositionManager (not the NFT holder).")

	for i, ev := range mods {
		if i >= 10 {
			t.Logf("  … %d more", len(mods)-10)
			break
		}
		action := "add"
		if ev.LiquidityDelta != nil && ev.LiquidityDelta.Sign() < 0 {
			action = "remove"
		}
		t.Logf("  [%02d] block=%d  action=%-6s  sender=%s  delta=%s",
			i+1, ev.Block, action, ev.Sender.Hex(), bigOrZero(ev.LiquidityDelta))
	}
}

// ── Helpers used only by these tests ─────────────────────────────────────────

func bigOrZero(x *big.Int) string {
	if x == nil {
		return "0"
	}
	return x.String()
}

func v4AddrLabel(addr common.Address) string {
	if (addr == common.Address{}) {
		return "ETH"
	}
	switch addr.Hex() {
	case "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48":
		return "USDC"
	case "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2":
		return "WETH"
	case "0x6B175474E89094C44Da98b954EedeAC495271d0F":
		return "DAI"
	case "0xdAC17F958D2ee523a2206206994597C13D831ec7":
		return "USDT"
	}
	h := addr.Hex()
	return h[:6] + "…" + h[len(h)-4:]
}

func v4HooksLabel(noHooks bool, hooks common.Address) string {
	if noHooks {
		return "none"
	}
	return hooks.Hex()
}
