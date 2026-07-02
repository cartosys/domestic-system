package helpers

import (
	"encoding/json"
	_ "embed"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

//go:embed data/ondo_gm_tokens.json
var ondoGMTokensJSON []byte

// OndoToken is a single Ondo Global Markets tokenized-stock entry from the
// vendored token list (see cmd/updateondotokens). It is compiled into the
// binary and immutable at runtime — distinct from config.WatchedToken/
// rpc.WatchedToken, which are user-editable and persisted to disk.
type OndoToken struct {
	Symbol   string
	Name     string
	Address  common.Address
	Decimals uint8
}

// OndoGMTokenList holds every Ondo Global Markets mainnet token from the
// vendored list, sorted by symbol.
var OndoGMTokenList []OndoToken

func init() {
	var raw struct {
		Tokens []struct {
			Symbol   string `json:"symbol"`
			Name     string `json:"name"`
			Address  string `json:"address"`
			Decimals uint8  `json:"decimals"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(ondoGMTokensJSON, &raw); err != nil {
		panic(fmt.Sprintf("helpers: malformed embedded ondo_gm_tokens.json: %v", err))
	}
	OndoGMTokenList = make([]OndoToken, 0, len(raw.Tokens))
	for _, t := range raw.Tokens {
		if !common.IsHexAddress(t.Address) {
			panic(fmt.Sprintf("helpers: invalid address %q for Ondo token %s in embedded data", t.Address, t.Symbol))
		}
		OndoGMTokenList = append(OndoGMTokenList, OndoToken{
			Symbol:   t.Symbol,
			Name:     t.Name,
			Address:  common.HexToAddress(t.Address),
			Decimals: t.Decimals,
		})
	}
}
