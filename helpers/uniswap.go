package helpers

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// UniswapV2Pair represents a Uniswap V2 pair contract
type UniswapV2Pair struct {
	Address common.Address
	Token0  common.Address
	Token1  common.Address
}

// SwapQuote represents the result of a swap price query
type SwapQuote struct {
	AmountIn       *big.Int // Input amount
	AmountOut      *big.Int // Expected output amount
	Token0Reserve  *big.Int // Reserve of token0
	Token1Reserve  *big.Int // Reserve of token1
	PriceImpact    float64  // Price impact percentage
	EffectivePrice float64  // Effective price (output/input)
}

// Uniswap V2 function selectors
var (
	// getReserves() returns (uint112 reserve0, uint112 reserve1, uint32 blockTimestampLast)
	getReservesSelector = []byte{0x09, 0x02, 0xf1, 0xac}
	// token0() returns (address)
	token0Selector = []byte{0x0d, 0xfe, 0x16, 0x81}
	// token1() returns (address)
	token1Selector = []byte{0xd2, 0x12, 0x20, 0xa7}
)

// Well-known Uniswap V2 pair addresses on Ethereum mainnet
var (
	// USDC/WETH pair on Uniswap V2 (this is the actual mainnet pair)
	// Verified at: https://v2.info.uniswap.org/pair/0xb4e16d0168e52d35cacd2c6185b44281ec28c9dc
	USDCWETHPairAddress = common.HexToAddress("0xB4e16d0168e52d35CaCD2c6185b44281Ec28C9Dc")
	// WETH address
	WETHAddress = common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	// USDC address
	USDCAddress = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	// DAI/WETH pair (for testing alternative)
	DAIWETHPairAddress = common.HexToAddress("0xA478c2975Ab1Ea89e8196811F51A7B7Ade33eB11")
	// DAI address
	DAIAddress = common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F")
)

// GetUniswapV2Pair fetches the token addresses for a Uniswap V2 pair
func GetUniswapV2Pair(client *ethclient.Client, pairAddress common.Address) (*UniswapV2Pair, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get token0
	token0Msg := ethereum.CallMsg{
		To:   &pairAddress,
		Data: token0Selector,
	}
	token0Data, err := client.CallContract(ctx, token0Msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get token0: %w", err)
	}
	if len(token0Data) != 32 {
		return nil, fmt.Errorf("token0 call returned unexpected data length: %d", len(token0Data))
	}

	// Get token1
	token1Msg := ethereum.CallMsg{
		To:   &pairAddress,
		Data: token1Selector,
	}
	token1Data, err := client.CallContract(ctx, token1Msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get token1: %w", err)
	}
	if len(token1Data) != 32 {
		return nil, fmt.Errorf("token1 call returned unexpected data length: %d", len(token1Data))
	}

	return &UniswapV2Pair{
		Address: pairAddress,
		Token0:  common.BytesToAddress(token0Data),
		Token1:  common.BytesToAddress(token1Data),
	}, nil
}

// fetchReserves returns (reserve0, reserve1) for pairAddress.
func fetchReserves(ctx context.Context, client *ethclient.Client, pairAddress common.Address) (reserve0, reserve1 *big.Int, err error) {
	msg := ethereum.CallMsg{To: &pairAddress, Data: getReservesSelector}
	data, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get reserves: %w", err)
	}
	if len(data) < 32 {
		return nil, nil, fmt.Errorf("invalid reserves data length: %d", len(data))
	}
	reserve0 = new(big.Int).SetBytes(data[0:32])
	reserve1 = big.NewInt(0)
	if len(data) >= 64 {
		reserve1 = new(big.Int).SetBytes(data[32:64])
	}
	return reserve0, reserve1, nil
}

// orderReserves returns (reserveIn, reserveOut) given which token is the input.
func orderReserves(pair *UniswapV2Pair, tokenIn common.Address, r0, r1 *big.Int) (reserveIn, reserveOut *big.Int, err error) {
	switch tokenIn {
	case pair.Token0:
		return r0, r1, nil
	case pair.Token1:
		return r1, r0, nil
	default:
		return nil, nil, fmt.Errorf("tokenIn %s not in pair (token0: %s, token1: %s)",
			tokenIn.Hex(), pair.Token0.Hex(), pair.Token1.Hex())
	}
}

// computePriceImpact returns the price impact percentage given spot and effective prices.
func computePriceImpact(reserveIn, reserveOut *big.Int, effectivePrice float64) float64 {
	if reserveIn.Sign() <= 0 || reserveOut.Sign() <= 0 {
		return 0
	}
	spotPrice := new(big.Float).Quo(new(big.Float).SetInt(reserveOut), new(big.Float).SetInt(reserveIn))
	spot, _ := spotPrice.Float64()
	if spot <= 0 {
		return 0
	}
	return ((spot - effectivePrice) / spot) * 100
}

// GetSwapQuote calculates the expected output amount for a swap using the Uniswap V2 formula.
// tokenIn is the token being sold; amountIn is the amount to sell.
func GetSwapQuote(client *ethclient.Client, pairAddress common.Address, tokenIn common.Address, amountIn *big.Int) (*SwapQuote, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pair, err := GetUniswapV2Pair(client, pairAddress)
	if err != nil {
		return nil, err
	}

	reserve0, reserve1, err := fetchReserves(ctx, client, pairAddress)
	if err != nil {
		return nil, err
	}

	reserveIn, reserveOut, err := orderReserves(pair, tokenIn, reserve0, reserve1)
	if err != nil {
		return nil, err
	}

	// amountOut = (amountIn * 997 * reserveOut) / (reserveIn * 1000 + amountIn * 997)
	amountInWithFee := new(big.Int).Mul(amountIn, big.NewInt(997))
	numerator := new(big.Int).Mul(amountInWithFee, reserveOut)
	denominator := new(big.Int).Add(new(big.Int).Mul(reserveIn, big.NewInt(1000)), amountInWithFee)
	amountOut := new(big.Int).Div(numerator, denominator)

	effectivePrice := 0.0
	if amountIn.Sign() > 0 {
		ef := new(big.Float).Quo(new(big.Float).SetInt(amountOut), new(big.Float).SetInt(amountIn))
		effectivePrice, _ = ef.Float64()
	}

	return &SwapQuote{
		AmountIn:       amountIn,
		AmountOut:      amountOut,
		Token0Reserve:  reserve0,
		Token1Reserve:  reserve1,
		PriceImpact:    computePriceImpact(reserveIn, reserveOut, effectivePrice),
		EffectivePrice: effectivePrice,
	}, nil
}

// GetReverseSwapQuote calculates the required input amount to receive a desired amountOut.
func GetReverseSwapQuote(client *ethclient.Client, pairAddress, tokenIn common.Address, amountOut *big.Int) (*SwapQuote, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pair, err := GetUniswapV2Pair(client, pairAddress)
	if err != nil {
		return nil, err
	}

	reserve0, reserve1, err := fetchReserves(ctx, client, pairAddress)
	if err != nil {
		return nil, err
	}

	reserveIn, reserveOut, err := orderReserves(pair, tokenIn, reserve0, reserve1)
	if err != nil {
		return nil, err
	}

	// amountIn = (reserveIn * amountOut * 1000) / ((reserveOut - amountOut) * 997) + 1
	denominator := new(big.Int).Sub(reserveOut, amountOut)
	denominator.Mul(denominator, big.NewInt(997))
	if denominator.Sign() <= 0 {
		return nil, fmt.Errorf("insufficient liquidity for desired output amount")
	}
	numerator := new(big.Int).Mul(new(big.Int).Mul(reserveIn, amountOut), big.NewInt(1000))
	amountIn := new(big.Int).Add(new(big.Int).Div(numerator, denominator), big.NewInt(1))

	effectivePrice := 0.0
	if amountIn.Sign() > 0 {
		ef := new(big.Float).Quo(new(big.Float).SetInt(amountOut), new(big.Float).SetInt(amountIn))
		effectivePrice, _ = ef.Float64()
	}

	return &SwapQuote{
		AmountIn:       amountIn,
		AmountOut:      amountOut,
		Token0Reserve:  reserve0,
		Token1Reserve:  reserve1,
		PriceImpact:    computePriceImpact(reserveIn, reserveOut, effectivePrice),
		EffectivePrice: effectivePrice,
	}, nil
}

// FormatSwapQuote returns a human-readable string for a swap quote
func FormatSwapQuote(quote *SwapQuote, tokenInSymbol, tokenOutSymbol string, tokenInDecimals, tokenOutDecimals uint8) string {
	if quote == nil {
		return "No quote available"
	}

	// Format amounts with proper decimals
	divisorIn := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokenInDecimals)), nil))
	amountInFormatted := new(big.Float).Quo(new(big.Float).SetInt(quote.AmountIn), divisorIn)

	divisorOut := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokenOutDecimals)), nil))
	amountOutFormatted := new(big.Float).Quo(new(big.Float).SetInt(quote.AmountOut), divisorOut)

	return fmt.Sprintf("%s %s → %s %s (impact: %.2f%%)",
		amountInFormatted.Text('f', 4), tokenInSymbol,
		amountOutFormatted.Text('f', 4), tokenOutSymbol,
		quote.PriceImpact)
}
