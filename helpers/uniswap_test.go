package helpers

import (
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
)

func TestGetSwapQuote_USDCtoETH(t *testing.T) {
	// Get RPC URL from environment
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping integration test")
	}

	// Connect to Ethereum
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Fatalf("Failed to connect to Ethereum: %v", err)
	}
	defer client.Close()

	// Test swapping 1000 USDC to WETH
	// USDC has 6 decimals, so 1000 USDC = 1000 * 10^6
	amountIn := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e6))

	// Get quote for USDC → WETH swap
	quote, err := GetSwapQuote(client, USDCWETHPairAddress, USDCAddress, amountIn)
	if err != nil {
		t.Fatalf("GetSwapQuote failed: %v", err)
	}

	// Verify quote is not nil
	if quote == nil {
		t.Fatal("Quote is nil")
	}

	// Verify we got a positive output amount
	if quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		t.Errorf("Expected positive output amount, got: %v", quote.AmountOut)
	}

	// Verify reserves are populated
	if quote.Token0Reserve == nil || quote.Token0Reserve.Sign() <= 0 {
		t.Errorf("Expected positive token0 reserve, got: %v", quote.Token0Reserve)
	}
	if quote.Token1Reserve == nil || quote.Token1Reserve.Sign() <= 0 {
		t.Errorf("Expected positive token1 reserve, got: %v", quote.Token1Reserve)
	}

	// Log the results
	t.Logf("Swap Quote for 1000 USDC → WETH:")
	t.Logf("  Amount In: %s USDC", new(big.Float).Quo(new(big.Float).SetInt(quote.AmountIn), big.NewFloat(1e6)).Text('f', 2))
	t.Logf("  Amount Out: %s WETH", new(big.Float).Quo(new(big.Float).SetInt(quote.AmountOut), big.NewFloat(1e18)).Text('f', 6))
	t.Logf("  Price Impact: %.4f%%", quote.PriceImpact)
	t.Logf("  Effective Price: %.6f WETH/USDC", quote.EffectivePrice)
	t.Logf("  Token0 Reserve: %s", quote.Token0Reserve.String())
	t.Logf("  Token1 Reserve: %s", quote.Token1Reserve.String())

	// Test the formatted output
	formatted := FormatSwapQuote(quote, "USDC", "WETH", 6, 18)
	t.Logf("  Formatted: %s", formatted)
}

func TestGetSwapQuote_ETHtoUSDC(t *testing.T) {
	// Get RPC URL from environment
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping integration test")
	}

	// Connect to Ethereum
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Fatalf("Failed to connect to Ethereum: %v", err)
	}
	defer client.Close()

	// Test swapping 1 WETH to USDC
	// WETH has 18 decimals, so 1 WETH = 1 * 10^18
	amountIn := new(big.Int).Mul(big.NewInt(1), big.NewInt(1e18))

	// Get quote for WETH → USDC swap
	quote, err := GetSwapQuote(client, USDCWETHPairAddress, WETHAddress, amountIn)
	if err != nil {
		t.Fatalf("GetSwapQuote failed: %v", err)
	}

	// Verify quote is not nil
	if quote == nil {
		t.Fatal("Quote is nil")
	}

	// Verify we got a positive output amount
	if quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		t.Errorf("Expected positive output amount, got: %v", quote.AmountOut)
	}

	// Log the results
	t.Logf("Swap Quote for 1 WETH → USDC:")
	t.Logf("  Amount In: %s WETH", new(big.Float).Quo(new(big.Float).SetInt(quote.AmountIn), big.NewFloat(1e18)).Text('f', 6))
	t.Logf("  Amount Out: %s USDC", new(big.Float).Quo(new(big.Float).SetInt(quote.AmountOut), big.NewFloat(1e6)).Text('f', 2))
	t.Logf("  Price Impact: %.4f%%", quote.PriceImpact)
	t.Logf("  Effective Price: %.6f USDC/WETH", quote.EffectivePrice)

	// Test the formatted output
	formatted := FormatSwapQuote(quote, "WETH", "USDC", 18, 6)
	t.Logf("  Formatted: %s", formatted)

	// Sanity check: 1 ETH should get us a reasonable amount of USDC (> $100)
	// This is a loose check in case prices change dramatically
	minExpectedUSDC := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e6)) // 100 USDC
	if quote.AmountOut.Cmp(minExpectedUSDC) < 0 {
		t.Errorf("Expected at least 100 USDC for 1 WETH, got: %s USDC", 
			new(big.Float).Quo(new(big.Float).SetInt(quote.AmountOut), big.NewFloat(1e6)).Text('f', 2))
	}
}

func TestGetUniswapV2Pair(t *testing.T) {
	// Get RPC URL from environment
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping integration test")
	}

	t.Logf("Using RPC: %s", rpcURL)
	// Test with DAI/WETH pair which is very well known
	t.Logf("Testing pair address: %s", DAIWETHPairAddress.Hex())

	// Connect to Ethereum
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Fatalf("Failed to connect to Ethereum: %v", err)
	}
	defer client.Close()

	t.Logf("Successfully connected to Ethereum")

	// Get pair info for DAI/WETH
	pair, err := GetUniswapV2Pair(client, DAIWETHPairAddress)
	if err != nil {
		t.Fatalf("GetUniswapV2Pair failed: %v", err)
	}

	// Verify pair address
	if pair.Address != DAIWETHPairAddress {
		t.Errorf("Expected pair address %s, got %s", DAIWETHPairAddress.Hex(), pair.Address.Hex())
	}

	// Verify tokens (should be WETH and DAI, order may vary)
	if (pair.Token0 != WETHAddress && pair.Token0 != DAIAddress) ||
		(pair.Token1 != WETHAddress && pair.Token1 != DAIAddress) {
		t.Errorf("Expected tokens to be WETH and DAI, got: token0=%s, token1=%s",
			pair.Token0.Hex(), pair.Token1.Hex())
	}

	// Verify token0 and token1 are different
	if pair.Token0 == pair.Token1 {
		t.Errorf("token0 and token1 should be different, both are: %s", pair.Token0.Hex())
	}

	t.Logf("Pair Info:")
	t.Logf("  Address: %s", pair.Address.Hex())
	t.Logf("  Token0: %s", pair.Token0.Hex())
	t.Logf("  Token1: %s", pair.Token1.Hex())
}

func TestGetSwapQuote_SmallAmount(t *testing.T) {
	// Get RPC URL from environment
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping integration test")
	}

	// Connect to Ethereum
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Fatalf("Failed to connect to Ethereum: %v", err)
	}
	defer client.Close()

	// Test swapping a very small amount: 1 USDC
	amountIn := big.NewInt(1e6) // 1 USDC (6 decimals)

	// Get quote for USDC → WETH swap
	quote, err := GetSwapQuote(client, USDCWETHPairAddress, USDCAddress, amountIn)
	if err != nil {
		t.Fatalf("GetSwapQuote failed: %v", err)
	}

	// Verify we got a positive output amount
	if quote.AmountOut == nil || quote.AmountOut.Sign() <= 0 {
		t.Errorf("Expected positive output amount for small swap, got: %v", quote.AmountOut)
	}

	// Price impact should be very small for such a small amount
	if quote.PriceImpact > 0.1 {
		t.Logf("Warning: Price impact %.4f%% seems high for 1 USDC", quote.PriceImpact)
	}

	t.Logf("Small Swap Quote for 1 USDC → WETH:")
	t.Logf("  Amount Out: %s WETH", new(big.Float).Quo(new(big.Float).SetInt(quote.AmountOut), big.NewFloat(1e18)).Text('f', 10))
	t.Logf("  Price Impact: %.6f%%", quote.PriceImpact)
}
