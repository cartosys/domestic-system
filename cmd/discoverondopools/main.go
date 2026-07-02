// Command discoverondopools scans the Uniswap V4 PoolManager's Initialize
// event log for pools involving an Ondo Global Markets token, and writes the
// result to helpers/data/ondo_v4_pools.json. V4 has no on-chain
// factory/registry to query a pool address from a token pair the way V2/V3
// do — a pool's existence is only knowable from having observed its
// Initialize event — so this vendored index (rebuilt by re-running this
// tool) is how helpers.ResolveOndoV4Pool answers that question quickly at
// runtime, with a bounded live fallback for anything created after the last
// rebuild (see helpers/uniswap_v4_quote.go).
//
// This is a maintainer-run dev tool, never invoked by the shipped TUI binary.
//
// Usage: ETH_RPC_URL=https://... go run ./cmd/discoverondopools
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/indexer"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const outPath = "helpers/data/ondo_v4_pools.json"

// scanWindowBlocks bounds how far back to scan. Ondo's V4 Global Markets
// launch is recent (days old at time of writing), so this comfortably covers
// it without an unbounded scan back to V4PoolManager's 2024 deploy block.
const scanWindowBlocks = 400_000

const chunkSize = uint64(10_000)

type outPoolEntry struct {
	OndoTokenSymbol string `json:"ondo_token_symbol"`
	OndoTokenAddr   string `json:"ondo_token_addr"`
	Currency0       string `json:"currency0"`
	Currency1       string `json:"currency1"`
	Fee             uint32 `json:"fee"`
	TickSpacing     int32  `json:"tick_spacing"`
	Hooks           string `json:"hooks"`
	PoolID          string `json:"pool_id"`
}

type outFile struct {
	FetchedAt  string         `json:"fetched_at"`
	FromBlock  uint64         `json:"from_block"`
	ToBlock    uint64         `json:"to_block"`
	Pools      []outPoolEntry `json:"pools"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "discoverondopools:", err)
		os.Exit(1)
	}
}

func run() error {
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		return fmt.Errorf("ETH_RPC_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return fmt.Errorf("get latest block: %w", err)
	}
	toBlock := header.Number.Uint64()
	fromBlock := uint64(0)
	if toBlock > scanWindowBlocks {
		fromBlock = toBlock - scanWindowBlocks
	}
	if fromBlock < indexer.V4DeployBlock {
		fromBlock = indexer.V4DeployBlock
	}

	ondoAddrs := make(map[common.Address]string, len(helpers.OndoGMTokenList))
	for _, t := range helpers.OndoGMTokenList {
		ondoAddrs[t.Address] = t.Symbol
	}
	// SPCXon is hardcoded elsewhere (helpers.UniswapNetworkAddresses.SPCXon)
	// but should still be discoverable here if it has a V4 pool.
	spcx := common.HexToAddress("0xc9eef266834730340A55B6CC24621B31BAF55581")
	ondoAddrs[spcx] = "SPCXon"

	var found []outPoolEntry
	seen := make(map[common.Hash]bool)

	for chunkFrom := fromBlock; chunkFrom <= toBlock; chunkFrom += chunkSize {
		chunkTo := chunkFrom + chunkSize - 1
		if chunkTo > toBlock {
			chunkTo = toBlock
		}
		events, err := indexer.FetchAllInitializeEvents(ctx, client, chunkFrom, chunkTo)
		if err != nil {
			return fmt.Errorf("scan blocks %d-%d: %w", chunkFrom, chunkTo, err)
		}
		for _, ev := range events {
			sym, isOndo := "", false
			if s, ok := ondoAddrs[ev.Currency0]; ok {
				sym, isOndo = s, true
			} else if s, ok := ondoAddrs[ev.Currency1]; ok {
				sym, isOndo = s, true
			}
			if !isOndo || seen[ev.PoolID] {
				continue
			}
			seen[ev.PoolID] = true

			fee := uint32(0)
			if ev.Fee != nil {
				fee = uint32(ev.Fee.Uint64())
			}
			tickSpacing := int32(0)
			if ev.TickSpacing != nil {
				tickSpacing = int32(ev.TickSpacing.Int64())
			}
			ondoAddr := ev.Currency0
			if ondoAddrs[ev.Currency1] == sym {
				ondoAddr = ev.Currency1
			}

			computedID := helpers.ComputePoolId(ev.Currency0, ev.Currency1, ev.Hooks, fee, tickSpacing)
			if computedID != ev.PoolID {
				fmt.Fprintf(os.Stderr, "discoverondopools: WARNING pool ID mismatch for %s — event=%s computed=%s (skipping)\n", sym, ev.PoolID.Hex(), computedID.Hex())
				continue
			}

			found = append(found, outPoolEntry{
				OndoTokenSymbol: sym,
				OndoTokenAddr:   ondoAddr.Hex(),
				Currency0:       ev.Currency0.Hex(),
				Currency1:       ev.Currency1.Hex(),
				Fee:             fee,
				TickSpacing:     tickSpacing,
				Hooks:           ev.Hooks.Hex(),
				PoolID:          ev.PoolID.Hex(),
			})
		}
		fmt.Fprintf(os.Stderr, "discoverondopools: scanned %d-%d, %d Ondo pools found so far\n", chunkFrom, chunkTo, len(found))
	}

	out := outFile{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Pools:     found,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("discoverondopools: wrote %d Ondo V4 pools to %s (blocks %d-%d)\n", len(found), outPath, fromBlock, toBlock)
	return nil
}
