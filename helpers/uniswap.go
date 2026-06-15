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
	Token0Reserve  *big.Int // Reserve of token0 (V2 only; zero for V3)
	Token1Reserve  *big.Int // Reserve of token1 (V2 only; zero for V3)
	PriceImpact    float64  // Price impact percentage
	EffectivePrice float64  // Effective price (output/input)
	IsV3           bool     // true when quote came from a Uniswap V3 pool
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

// SepoliaChainID is the chain ID of the Sepolia testnet (EIP-155).
const SepoliaChainID = 11155111

// UniswapNetworkAddresses bundles the Uniswap V2 router/factory contract
// addresses plus the watched-token and reference-pair addresses to use on a
// given network. UniswapAddressesForChain is the single place that decides
// which set applies.
type UniswapNetworkAddresses struct {
	Router  common.Address
	Factory common.Address

	WETH  common.Address
	USDC  common.Address
	USDT  common.Address
	DAI   common.Address
	SPCXon common.Address // SpaceX (Ondo Tokenized) — mainnet only

	USDCWETHPair   common.Address
	DAIWETHPair    common.Address
	USDTWETHPair   common.Address
	SPCXonUSDCPair common.Address // USDC/SPCXon V2 pair — mainnet only

	// Uniswap V3
	QuoterV2        common.Address // QuoterV2 for off-chain quote simulation
	SwapRouterV3    common.Address // SwapRouter02
	SPCXonUSDCPoolV3 common.Address // SPCXon/USDC 1% V3 pool — mainnet only
	SPCXonUSDTPoolV3 common.Address // SPCXon/USDT 0.3% V3 pool — mainnet only
}

// mainnetUniswapAddresses mirrors the package-level mainnet vars above so the
// live-RPC tests in uniswap_test.go (which run against mainnet) keep working
// against the same addresses. Router/Factory/USDT were previously inlined at
// their call sites (commands.go, model.go) — named here for symmetry with Sepolia.
var mainnetUniswapAddresses = UniswapNetworkAddresses{
	Router:  common.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"),
	Factory: common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"),

	WETH:   WETHAddress,
	USDC:   USDCAddress,
	USDT:   common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7"),
	DAI:    DAIAddress,
	SPCXon: common.HexToAddress("0xc9eef266834730340A55B6CC24621B31BAF55581"),

	USDCWETHPair:   USDCWETHPairAddress,
	DAIWETHPair:    DAIWETHPairAddress,
	SPCXonUSDCPair: common.HexToAddress("0x3fc51ce94bc6dd3cdfe599f1f99c05a5cc90e059"),

	QuoterV2:         common.HexToAddress("0x61fFE014bA17989E743c5F6cB21bF9697530B21e"),
	SwapRouterV3:     common.HexToAddress("0x68b3465833fb72A70ecDF485E0e4C7bD8665Fc45"),
	SPCXonUSDCPoolV3: common.HexToAddress("0x0461c60ad5fc24cb1fc075b7f202095819de6944"),
	SPCXonUSDTPoolV3: common.HexToAddress("0xe88f804369cf4274207eb26fc801b6f2df10ec4b"),
}

// sepoliaUniswapAddresses holds the Uniswap V2 deployment and token addresses
// on Sepolia (chain ID 11155111). Verified directly on-chain against a public
// Sepolia RPC: router.factory()/router.WETH() match the listed Factory/WETH,
// each token's symbol()/decimals() match, and each pair has live non-zero
// reserves on the Factory's pair list.
var sepoliaUniswapAddresses = UniswapNetworkAddresses{
	Router:  common.HexToAddress("0xeE567Fe1712Faf6149d80dA1E6934E354124CfE3"),
	Factory: common.HexToAddress("0xF62c03E08ada871A0bEb309762E260a7a6a880E6"),

	WETH: common.HexToAddress("0xfFf9976782d46CC05630D1f6eBAB18b2324d6B14"),
	USDC: common.HexToAddress("0x94a9D9AC8a22534E3FaCa9F4e7F2E2cf85d5E4C8"),
	USDT: common.HexToAddress("0xaa8E23Fb1079EA71e0a56F48a2aa51851D8433D0"),
	DAI:  common.HexToAddress("0xB4F1737Af37711e9A5890D9510c9bB60e170CB0D"),

	USDCWETHPair: common.HexToAddress("0x06D1080CDcbf8aD77a65a40F4484E93eA6180269"),
	DAIWETHPair:  common.HexToAddress("0x04ef46A6FAFc277dE43AAd0eF17d14Fb967e64B3"),
	USDTWETHPair: common.HexToAddress("0xcbDB9cb0669906c8B12211824B4F069D183155fF"),
}

// UniswapAddressesForChain returns the Uniswap V2 router/factory/token/pair
// addresses to use for chainID. Nil or unrecognized chain IDs default to
// Ethereum mainnet, matching the app's existing default RPC/network.
func UniswapAddressesForChain(chainID *big.Int) UniswapNetworkAddresses {
	if chainID != nil && chainID.Cmp(big.NewInt(SepoliaChainID)) == 0 {
		return sepoliaUniswapAddresses
	}
	return mainnetUniswapAddresses
}

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
