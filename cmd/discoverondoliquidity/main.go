// Command discoverondoliquidity checks every Ondo Global Markets token
// (helpers.OndoGMTokenList, plus the pre-existing hardcoded SPCXon) against
// USDC/WETH/USDT using the same on-chain resolution the app uses at runtime
// (helpers.ResolvePairOnChain — V2/V3/V4, liquidity-gated), and writes the
// tokens that currently have a live pool to helpers/data/ondo_liquid_tokens.json.
// That file seeds the default token watchlist (model_helpers.go's
// buildTokenWatchlist) for new configs.
//
// Liquidity shifts over time — pools gain and lose it, new pools appear —
// so this is meant to be re-run periodically, not a one-time snapshot.
//
// This is a maintainer-run dev tool, never invoked by the shipped TUI binary.
//
// Usage: ETH_RPC_URL=https://... go run ./cmd/discoverondoliquidity
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"charm-wallet-tui/helpers"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const outPath = "helpers/data/ondo_liquid_tokens.json"

type outToken struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name,omitempty"`
	Address  string `json:"address"`
	Decimals uint8  `json:"decimals"`
}

type outFile struct {
	FetchedAt string     `json:"fetched_at"`
	Tokens    []outToken `json:"tokens"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "discoverondoliquidity:", err)
		os.Exit(1)
	}
}

func run() error {
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		return fmt.Errorf("ETH_RPC_URL is not set")
	}
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	addrs := helpers.UniswapAddressesForChain(nil)

	tokens := make([]helpers.OndoToken, len(helpers.OndoGMTokenList))
	copy(tokens, helpers.OndoGMTokenList)
	if addrs.SPCXon != (common.Address{}) {
		tokens = append(tokens, helpers.OndoToken{Symbol: "SPCXon", Decimals: 18, Address: addrs.SPCXon})
	}

	quoteCurrencies := []common.Address{addrs.USDC, addrs.WETH, addrs.USDT}

	var liquid []outToken
	checked := 0
	for _, t := range tokens {
		hasLiquidity := false
		for _, q := range quoteCurrencies {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			_, rerr := helpers.ResolvePairOnChain(ctx, client, addrs, t.Address, q)
			cancel()
			checked++
			if rerr == nil {
				hasLiquidity = true
				break
			}
		}
		if hasLiquidity {
			liquid = append(liquid, outToken{Symbol: t.Symbol, Name: t.Name, Address: t.Address.Hex(), Decimals: t.Decimals})
			fmt.Fprintf(os.Stderr, "discoverondoliquidity: %s has live liquidity\n", t.Symbol)
		}
		if checked%300 == 0 {
			fmt.Fprintf(os.Stderr, "discoverondoliquidity: checked %d/%d combos, %d liquid tokens so far\n", checked, len(tokens)*len(quoteCurrencies), len(liquid))
		}
	}

	out := outFile{FetchedAt: time.Now().UTC().Format(time.RFC3339), Tokens: liquid}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("discoverondoliquidity: wrote %d liquid tokens to %s\n", len(liquid), outPath)
	return nil
}
