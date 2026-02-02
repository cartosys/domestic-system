# Uniswap Helper Functions

This package provides helper functions for interacting with Uniswap V2 pairs using RPC calls.

## Features

- **GetUniswapV2Pair**: Fetches token addresses for a Uniswap V2 pair contract
- **GetSwapQuote**: Calculates expected output amounts for swaps using the Uniswap V2 constant product formula
- **FormatSwapQuote**: Formats swap quotes in a human-readable format

## Usage

### Get Swap Quote for USDC → WETH

```go
import (
    "math/big"
    "github.com/ethereum/go-ethereum/ethclient"
    "charm-wallet-tui/helpers"
)

// Connect to Ethereum RPC
client, err := ethclient.Dial("https://eth.drpc.org")
if err != nil {
    panic(err)
}
defer client.Close()

// Swap 1000 USDC to WETH
amountIn := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e6)) // 1000 USDC (6 decimals)

quote, err := helpers.GetSwapQuote(
    client,
    helpers.USDCWETHPairAddress,
    helpers.USDCAddress,
    amountIn,
)
if err != nil {
    panic(err)
}

// Format the quote
formatted := helpers.FormatSwapQuote(quote, "USDC", "WETH", 6, 18)
fmt.Println(formatted) // Output: "1000.0000 USDC → 0.4268 WETH (impact: 0.31%)"
```

### Get Swap Quote for WETH → USDC

```go
// Swap 1 WETH to USDC
amountIn := new(big.Int).Mul(big.NewInt(1), big.NewInt(1e18)) // 1 WETH (18 decimals)

quote, err := helpers.GetSwapQuote(
    client,
    helpers.USDCWETHPairAddress,
    helpers.WETHAddress,
    amountIn,
)
if err != nil {
    panic(err)
}

formatted := helpers.FormatSwapQuote(quote, "WETH", "USDC", 18, 6)
fmt.Println(formatted) // Output: "1.0000 WETH → 2328.0658 USDC (impact: 0.32%)"
```

### Get Pair Information

```go
pair, err := helpers.GetUniswapV2Pair(client, helpers.USDCWETHPairAddress)
if err != nil {
    panic(err)
}

fmt.Printf("Pair: %s\n", pair.Address.Hex())
fmt.Printf("Token0: %s\n", pair.Token0.Hex())
fmt.Printf("Token1: %s\n", pair.Token1.Hex())
```

## SwapQuote Structure

```go
type SwapQuote struct {
    AmountIn       *big.Int // Input amount
    AmountOut      *big.Int // Expected output amount
    Token0Reserve  *big.Int // Reserve of token0 in the pair
    Token1Reserve  *big.Int // Reserve of token1 in the pair
    PriceImpact    float64  // Price impact percentage
    EffectivePrice float64  // Effective price (output/input)
}
```

## Pre-configured Pairs

The package includes addresses for well-known Uniswap V2 pairs on Ethereum mainnet:

- **USDCWETHPairAddress**: USDC/WETH pair
- **DAIWETHPairAddress**: DAI/WETH pair
- **USDCAddress**: USDC token
- **WETHAddress**: WETH token
- **DAIAddress**: DAI token

## How It Works

The helper uses direct RPC calls to query Uniswap V2 pair contracts:

1. **token0()** and **token1()**: Fetches the two token addresses in the pair
2. **getReserves()**: Fetches current liquidity reserves
3. Applies the Uniswap V2 constant product formula: `amountOut = (amountIn * 997 * reserveOut) / (reserveIn * 1000 + amountIn * 997)`
   - The 997/1000 factor accounts for the 0.3% trading fee

## Testing

Run the tests with an Ethereum RPC endpoint:

```bash
ETH_RPC_URL="https://eth.drpc.org" go test ./helpers -v -run "TestGetSwapQuote|TestGetUniswapV2Pair"
```

Tests include:
- USDC → WETH swap quote
- WETH → USDC swap quote
- Small amount swap (price impact validation)
- Pair information retrieval

## Notes

- All quotes are estimates based on current reserves and do not account for:
  - Slippage tolerance
  - Other transactions in the same block
  - MEV attacks
- Quotes include the 0.3% Uniswap V2 trading fee
- Price impact is calculated as the difference between spot price and effective execution price
