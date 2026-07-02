package helpers

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"charm-wallet-tui/indexer"
)

// PoolVersion identifies which Uniswap version a resolved pool belongs to.
type PoolVersion uint8

const (
	PoolVersionV2 PoolVersion = iota
	PoolVersionV3
	PoolVersionV4
)

// V4PoolKey mirrors Uniswap V4's on-chain PoolKey struct.
type V4PoolKey struct {
	Currency0   common.Address
	Currency1   common.Address
	Hooks       common.Address
	Fee         uint32
	TickSpacing int32
}

// ResolvedPool carries the routing metadata for whichever Uniswap version
// ResolvePairOnChain found liquidity on. Exactly one of {PairAddr+V3Fee,
// V4Key+V4PoolID} is populated, selected by Version.
type ResolvedPool struct {
	Version  PoolVersion
	PairAddr common.Address // V2 pair / V3 pool contract; zero for V4
	V3Fee    uint32
	V4Key    V4PoolKey
	V4PoolID common.Hash
}

// v4LiveFallbackWindowBlocks bounds the live-scan fallback in resolveV4Pool
// to the same recency window FetchPoolKey already uses elsewhere in this
// package, staying under the common ~100k-block eth_getLogs range limit.
const v4LiveFallbackWindowBlocks = 99000

// resolveV4Pool finds a V4 pool for tokenA/tokenB. V4 has no on-chain
// factory/registry to query live the way V2's getPair/V3's getPool do — a
// pool's existence is only knowable from having observed its Initialize
// event — so this uses a two-tier approach:
//
//  1. The vendored discovery index (ResolveOndoV4Pool), built offline by
//     cmd/discoverondopools. Instant, zero RPC calls, covers this feature's
//     actual scope (Ondo Global Markets tokens).
//  2. A bounded live fallback scanning the most recent ~99,000 blocks for an
//     Initialize event matching tokenA/tokenB, for pools created after the
//     last index rebuild. NOT exhaustive — a matching pool older than the
//     window and missing from the vendored index will not be found.
func resolveV4Pool(ctx context.Context, client *ethclient.Client, addrs UniswapNetworkAddresses, tokenA, tokenB common.Address) (ResolvedPool, bool) {
	if entry, ok := ResolveOndoV4Pool(tokenA, tokenB); ok {
		return ResolvedPool{
			Version: PoolVersionV4,
			V4Key: V4PoolKey{
				Currency0:   entry.Currency0,
				Currency1:   entry.Currency1,
				Hooks:       entry.Hooks,
				Fee:         entry.Fee,
				TickSpacing: entry.TickSpacing,
			},
			V4PoolID: entry.PoolID,
		}, true
	}

	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return ResolvedPool{}, false
	}
	toBlock := header.Number.Uint64()
	fromBlock := uint64(0)
	if toBlock > v4LiveFallbackWindowBlocks {
		fromBlock = toBlock - v4LiveFallbackWindowBlocks
	}
	if fromBlock < indexer.V4DeployBlock {
		fromBlock = indexer.V4DeployBlock
	}

	events, err := indexer.FetchAllInitializeEvents(ctx, client, fromBlock, toBlock)
	if err != nil {
		return ResolvedPool{}, false
	}

	stateViewABI, abiErr := abi.JSON(strings.NewReader(poolManagerViewABI))
	for _, ev := range events {
		matches := (ev.Currency0 == tokenA && ev.Currency1 == tokenB) || (ev.Currency0 == tokenB && ev.Currency1 == tokenA)
		if !matches {
			continue
		}
		// Only accept if the pool has liquidity, mirroring the V3 tier's
		// "don't route to a dead pool" behavior in ResolvePairOnChain.
		if abiErr != nil || addrs.V4StateView == (common.Address{}) {
			continue
		}
		_, _, _, _, liquidity, lerr := v4GetSlot0(ctx, client, &stateViewABI, addrs.V4StateView, ev.PoolID)
		if lerr != nil || liquidity == nil || liquidity.Sign() <= 0 {
			continue
		}
		fee := uint32(0)
		if ev.Fee != nil {
			fee = uint32(ev.Fee.Uint64())
		}
		tickSpacing := int32(0)
		if ev.TickSpacing != nil {
			tickSpacing = int32(ev.TickSpacing.Int64())
		}
		return ResolvedPool{
			Version: PoolVersionV4,
			V4Key: V4PoolKey{
				Currency0:   ev.Currency0,
				Currency1:   ev.Currency1,
				Hooks:       ev.Hooks,
				Fee:         fee,
				TickSpacing: tickSpacing,
			},
			V4PoolID: ev.PoolID,
		}, true
	}

	return ResolvedPool{}, false
}

// ---- V4Quoter ----

const v4QuoterABI = `[
  {
    "inputs": [{
      "name": "params", "type": "tuple",
      "components": [
        {"name": "poolKey", "type": "tuple", "components": [
          {"name": "currency0", "type": "address"},
          {"name": "currency1", "type": "address"},
          {"name": "fee", "type": "uint24"},
          {"name": "tickSpacing", "type": "int24"},
          {"name": "hooks", "type": "address"}
        ]},
        {"name": "zeroForOne", "type": "bool"},
        {"name": "exactAmount", "type": "uint128"},
        {"name": "hookData", "type": "bytes"}
      ]
    }],
    "name": "quoteExactInputSingle",
    "outputs": [
      {"name": "amountOut", "type": "uint256"},
      {"name": "gasEstimate", "type": "uint256"}
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [{
      "name": "params", "type": "tuple",
      "components": [
        {"name": "poolKey", "type": "tuple", "components": [
          {"name": "currency0", "type": "address"},
          {"name": "currency1", "type": "address"},
          {"name": "fee", "type": "uint24"},
          {"name": "tickSpacing", "type": "int24"},
          {"name": "hooks", "type": "address"}
        ]},
        {"name": "zeroForOne", "type": "bool"},
        {"name": "exactAmount", "type": "uint128"},
        {"name": "hookData", "type": "bytes"}
      ]
    }],
    "name": "quoteExactOutputSingle",
    "outputs": [
      {"name": "amountIn", "type": "uint256"},
      {"name": "gasEstimate", "type": "uint256"}
    ],
    "stateMutability": "nonpayable",
    "type": "function"
  }
]`

type v4QuoterPoolKeyABI struct {
	Currency0   common.Address
	Currency1   common.Address
	Fee         *big.Int
	TickSpacing *big.Int
	Hooks       common.Address
}

type v4QuoterParamsABI struct {
	PoolKey     v4QuoterPoolKeyABI
	ZeroForOne  bool
	ExactAmount *big.Int
	HookData    []byte
}

// v4QuotePriceImpact computes price impact for a V4 quote using the pool's
// current tick as spot price, reusing priceImpactFromSpot (the same tail
// formula V2's computePriceImpact uses) since V4Quoter doesn't return a
// before/after sqrtPriceX96 pair the way V3's QuoterV2 does.
func v4QuotePriceImpact(ctx context.Context, client *ethclient.Client, addrs UniswapNetworkAddresses, key V4PoolKey, poolID common.Hash, zeroForOne bool, amountIn, amountOut *big.Int) float64 {
	if addrs.V4StateView == (common.Address{}) {
		return 0
	}
	stateViewABI, err := abi.JSON(strings.NewReader(poolManagerViewABI))
	if err != nil {
		return 0
	}
	_, tick, _, _, _, err := v4GetSlot0(ctx, client, &stateViewABI, addrs.V4StateView, poolID)
	if err != nil {
		return 0
	}
	spot := v4RawSpotPrice(tick) // token1_raw per token0_raw
	if !zeroForOne {
		if spot == 0 {
			return 0
		}
		spot = 1 / spot
	}
	if amountIn == nil || amountIn.Sign() <= 0 {
		return 0
	}
	effective, _ := new(big.Float).Quo(new(big.Float).SetInt(amountOut), new(big.Float).SetInt(amountIn)).Float64()
	return priceImpactFromSpot(spot, effective)
}

// GetV4SwapQuote fetches an exact-input quote from the Uniswap V4Quoter.
func GetV4SwapQuote(client *ethclient.Client, addrs UniswapNetworkAddresses, key V4PoolKey, poolID common.Hash, tokenIn common.Address, amountIn *big.Int) (*SwapQuote, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	parsedABI, err := abi.JSON(strings.NewReader(v4QuoterABI))
	if err != nil {
		return nil, fmt.Errorf("parse V4Quoter ABI: %w", err)
	}

	zeroForOne := tokenIn == key.Currency0
	params := v4QuoterParamsABI{
		PoolKey: v4QuoterPoolKeyABI{
			Currency0:   key.Currency0,
			Currency1:   key.Currency1,
			Fee:         new(big.Int).SetUint64(uint64(key.Fee)),
			TickSpacing: big.NewInt(int64(key.TickSpacing)),
			Hooks:       key.Hooks,
		},
		ZeroForOne:  zeroForOne,
		ExactAmount: amountIn,
		HookData:    []byte{},
	}
	calldata, err := parsedABI.Pack("quoteExactInputSingle", params)
	if err != nil {
		return nil, fmt.Errorf("pack quoteExactInputSingle: %w", err)
	}
	quoter := addrs.V4Quoter
	raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: calldata}, nil)
	if err != nil {
		return nil, fmt.Errorf("quoteExactInputSingle failed: %w", err)
	}
	vals, err := parsedABI.Unpack("quoteExactInputSingle", raw)
	if err != nil || len(vals) == 0 {
		return nil, fmt.Errorf("unpack quoteExactInputSingle: %w", err)
	}
	amountOut := vals[0].(*big.Int)

	effectivePrice := 0.0
	if amountIn.Sign() > 0 {
		ep := new(big.Float).Quo(new(big.Float).SetInt(amountOut), new(big.Float).SetInt(amountIn))
		effectivePrice, _ = ep.Float64()
	}

	return &SwapQuote{
		AmountIn:       amountIn,
		AmountOut:      amountOut,
		PriceImpact:    v4QuotePriceImpact(ctx, client, addrs, key, poolID, zeroForOne, amountIn, amountOut),
		EffectivePrice: effectivePrice,
		IsV4:           true,
		HookAddr:       key.Hooks,
	}, nil
}

// GetV4ReverseSwapQuote fetches an exact-output quote from the Uniswap V4Quoter.
func GetV4ReverseSwapQuote(client *ethclient.Client, addrs UniswapNetworkAddresses, key V4PoolKey, poolID common.Hash, tokenIn common.Address, amountOut *big.Int) (*SwapQuote, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	parsedABI, err := abi.JSON(strings.NewReader(v4QuoterABI))
	if err != nil {
		return nil, fmt.Errorf("parse V4Quoter ABI: %w", err)
	}

	zeroForOne := tokenIn == key.Currency0
	params := v4QuoterParamsABI{
		PoolKey: v4QuoterPoolKeyABI{
			Currency0:   key.Currency0,
			Currency1:   key.Currency1,
			Fee:         new(big.Int).SetUint64(uint64(key.Fee)),
			TickSpacing: big.NewInt(int64(key.TickSpacing)),
			Hooks:       key.Hooks,
		},
		ZeroForOne:  zeroForOne,
		ExactAmount: amountOut,
		HookData:    []byte{},
	}
	calldata, err := parsedABI.Pack("quoteExactOutputSingle", params)
	if err != nil {
		return nil, fmt.Errorf("pack quoteExactOutputSingle: %w", err)
	}
	quoter := addrs.V4Quoter
	raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &quoter, Data: calldata}, nil)
	if err != nil {
		return nil, fmt.Errorf("quoteExactOutputSingle failed: %w", err)
	}
	vals, err := parsedABI.Unpack("quoteExactOutputSingle", raw)
	if err != nil || len(vals) == 0 {
		return nil, fmt.Errorf("unpack quoteExactOutputSingle: %w", err)
	}
	amountIn := vals[0].(*big.Int)

	effectivePrice := 0.0
	if amountIn.Sign() > 0 {
		ep := new(big.Float).Quo(new(big.Float).SetInt(amountOut), new(big.Float).SetInt(amountIn))
		effectivePrice, _ = ep.Float64()
	}

	return &SwapQuote{
		AmountIn:       amountIn,
		AmountOut:      amountOut,
		PriceImpact:    v4QuotePriceImpact(ctx, client, addrs, key, poolID, zeroForOne, amountIn, amountOut),
		EffectivePrice: effectivePrice,
		IsV4:           true,
		HookAddr:       key.Hooks,
	}, nil
}
