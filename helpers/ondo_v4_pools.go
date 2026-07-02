package helpers

import (
	"encoding/json"
	_ "embed"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

//go:embed data/ondo_v4_pools.json
var ondoV4PoolsJSON []byte

// OndoPoolEntry is a discovered Uniswap V4 pool involving an Ondo Global
// Markets token, produced by cmd/discoverondopools. V4 has no on-chain
// factory/registry to look this up live (unlike V2/V3's getPair/getPool) —
// a pool is only knowable from having observed its Initialize event — so
// this vendored index is the primary lookup ResolveOndoV4Pool uses; see
// resolveV4Pool in uniswap_v4_quote.go for the bounded live fallback that
// covers pools created after the last index rebuild.
type OndoPoolEntry struct {
	OndoTokenSymbol string
	OndoTokenAddr   common.Address
	Currency0       common.Address
	Currency1       common.Address
	Fee             uint32
	TickSpacing     int32
	Hooks           common.Address
	PoolID          common.Hash
}

// OndoV4Pools holds every discovered Ondo Global Markets V4 pool from the
// vendored index. Empty until cmd/discoverondopools has been run against a
// live RPC endpoint — see ondo_v4_pools.json's fetched_at field.
var OndoV4Pools []OndoPoolEntry

func init() {
	var raw struct {
		Pools []struct {
			OndoTokenSymbol string `json:"ondo_token_symbol"`
			OndoTokenAddr   string `json:"ondo_token_addr"`
			Currency0       string `json:"currency0"`
			Currency1       string `json:"currency1"`
			Fee             uint32 `json:"fee"`
			TickSpacing     int32  `json:"tick_spacing"`
			Hooks           string `json:"hooks"`
			PoolID          string `json:"pool_id"`
		} `json:"pools"`
	}
	if err := json.Unmarshal(ondoV4PoolsJSON, &raw); err != nil {
		panic(fmt.Sprintf("helpers: malformed embedded ondo_v4_pools.json: %v", err))
	}
	OndoV4Pools = make([]OndoPoolEntry, 0, len(raw.Pools))
	for _, p := range raw.Pools {
		if !common.IsHexAddress(p.OndoTokenAddr) || !common.IsHexAddress(p.Currency0) || !common.IsHexAddress(p.Currency1) {
			panic(fmt.Sprintf("helpers: invalid address in embedded ondo_v4_pools.json entry for %s", p.OndoTokenSymbol))
		}
		OndoV4Pools = append(OndoV4Pools, OndoPoolEntry{
			OndoTokenSymbol: p.OndoTokenSymbol,
			OndoTokenAddr:   common.HexToAddress(p.OndoTokenAddr),
			Currency0:       common.HexToAddress(p.Currency0),
			Currency1:       common.HexToAddress(p.Currency1),
			Fee:             p.Fee,
			TickSpacing:     p.TickSpacing,
			Hooks:           common.HexToAddress(p.Hooks),
			PoolID:          common.HexToHash(p.PoolID),
		})
	}
}

// ResolveOndoV4Pool returns the vendored pool entry pairing tokenA and
// tokenB (in either order), ok=false if not indexed.
func ResolveOndoV4Pool(tokenA, tokenB common.Address) (OndoPoolEntry, bool) {
	for _, p := range OndoV4Pools {
		if (p.Currency0 == tokenA && p.Currency1 == tokenB) || (p.Currency0 == tokenB && p.Currency1 == tokenA) {
			return p, true
		}
	}
	return OndoPoolEntry{}, false
}
