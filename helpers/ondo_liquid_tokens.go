package helpers

import (
	"encoding/json"
	_ "embed"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

//go:embed data/ondo_liquid_tokens.json
var ondoLiquidTokensJSON []byte

// OndoLiquidTokens holds the subset of OndoGMTokenList confirmed (by
// cmd/discoverondoliquidity) to have a live, liquid V2/V3/V4 pool against
// USDC/WETH/USDT as of the vendored data's fetched_at time. Used to seed the
// default token watchlist (model_helpers.go's buildTokenWatchlist) — liquidity
// shifts over time, so this should be periodically refreshed by re-running
// that tool, not treated as permanently authoritative.
var OndoLiquidTokens []OndoToken

func init() {
	var raw struct {
		Tokens []struct {
			Symbol   string `json:"symbol"`
			Name     string `json:"name"`
			Address  string `json:"address"`
			Decimals uint8  `json:"decimals"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(ondoLiquidTokensJSON, &raw); err != nil {
		panic(fmt.Sprintf("helpers: malformed embedded ondo_liquid_tokens.json: %v", err))
	}
	OndoLiquidTokens = make([]OndoToken, 0, len(raw.Tokens))
	for _, t := range raw.Tokens {
		if !common.IsHexAddress(t.Address) {
			panic(fmt.Sprintf("helpers: invalid address %q for Ondo token %s in embedded data", t.Address, t.Symbol))
		}
		OndoLiquidTokens = append(OndoLiquidTokens, OndoToken{
			Symbol:   t.Symbol,
			Name:     t.Name,
			Address:  common.HexToAddress(t.Address),
			Decimals: t.Decimals,
		})
	}
}
