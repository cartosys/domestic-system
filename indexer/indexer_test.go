package indexer

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

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
			helpers.HyperAddr(ev.From),
			helpers.HyperAddr(ev.To),
			ev.Value.String(),
			ev.Block,
			helpers.HyperTxHash(ev.TxHash),
		)
	}

	// ── Filtered query using the provided topic hash ──────────────────────────
	t.Logf("Filtered: querying for address %s (topic %s)",
		helpers.HyperAddr(testAddr),
		helpers.HyperTxHash(testAddrTopic),
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
		t.Logf("  Token   : %s (%s)", ev.Symbol, helpers.HyperAddr(ev.Token))
		t.Logf("  Block   : %d", ev.Block)
		t.Logf("  TxHash  : %s", helpers.HyperTxHash(ev.TxHash))
		t.Logf("  LogIndex: %d", ev.LogIndex)
		t.Logf("  From    : %s", helpers.HyperAddr(ev.From))
		t.Logf("  To      : %s", helpers.HyperAddr(ev.To))
		t.Logf("  Value   : %s raw  (%s %s)", ev.Value.String(), fmt.Sprintf("%.6f", humanAmt), ev.Symbol)
		t.Logf("  Decimals: %d", ev.Decimals)
		t.Logf("─────────────────────────────────────────────────────────")
	}

	if len(events) == 0 {
		t.Errorf("expected at least one USDC transfer at block %d for address %s, got none", testBlock, testAddr.Hex())
	}
}
