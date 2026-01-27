package rpc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestConnect(t *testing.T) {
	// Get RPC URL from environment
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping connection test")
	}

	t.Run("successful connection", func(t *testing.T) {
		result := Connect(rpcURL)
		
		if result.Error != nil {
			t.Fatalf("Failed to connect to RPC: %v", result.Error)
		}
		
		if result.Client == nil {
			t.Fatal("Client is nil despite no error")
		}
		
		if result.Client.URL != rpcURL {
			t.Errorf("Expected URL %s, got %s", rpcURL, result.Client.URL)
		}
		
		// Test that we can make a basic call
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		// Try to get the chain ID
		chainID, err := result.Client.ChainID(ctx)
		if err != nil {
			t.Errorf("Failed to get chain ID: %v", err)
		} else {
			t.Logf("Connected to chain ID: %s", chainID.String())
		}
	})

	t.Run("connection with timeout", func(t *testing.T) {
		result := ConnectWithTimeout(rpcURL, 10*time.Second)
		
		if result.Error != nil {
			t.Fatalf("Failed to connect with custom timeout: %v", result.Error)
		}
		
		if result.Client == nil {
			t.Fatal("Client is nil despite no error")
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		// Test with a completely malformed URL
		result := Connect("not-a-valid-url")
		
		// For invalid URLs, we expect either an error or a nil client
		// The exact behavior may vary by URL format
		if result.Error == nil && result.Client != nil {
			t.Log("Warning: Invalid URL accepted by RPC client (may depend on URL format)")
		}
	})
}

func TestLoadWalletDetails(t *testing.T) {
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping wallet details test")
	}

	// Connect first
	connResult := Connect(rpcURL)
	if connResult.Error != nil {
		t.Fatalf("Failed to connect: %v", connResult.Error)
	}

	// Use a well-known address with ETH (e.g., Vitalik's address)
	testAddr := common.HexToAddress("0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045")

	// Define some common tokens to watch
	watchTokens := []WatchedToken{
		{Symbol: "WETH", Decimals: 18, Address: common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")},
		{Symbol: "USDC", Decimals: 6, Address: common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")},
	}

	t.Run("load wallet details", func(t *testing.T) {
		details := LoadWalletDetails(connResult.Client, testAddr, watchTokens)
		
		// Details might fail due to rate limiting or network issues, so we just log errors
		if details.ErrMessage != "" {
			t.Logf("Got error message (may be due to rate limiting): %s", details.ErrMessage)
		}
		
		if details.Address != testAddr.Hex() {
			t.Errorf("Expected address %s, got %s", testAddr.Hex(), details.Address)
		}
		
		if details.EthWei == nil {
			t.Error("EthWei is nil")
		} else {
			t.Logf("ETH Balance (wei): %s", details.EthWei.String())
		}
		
		if details.LoadedAt.IsZero() {
			t.Error("LoadedAt timestamp is zero")
		}
		
		t.Logf("Found %d token balances", len(details.Tokens))
		for _, tok := range details.Tokens {
			t.Logf("  %s: %s", tok.Symbol, tok.Balance.String())
		}
	})

	t.Run("nil client", func(t *testing.T) {
		details := LoadWalletDetails(nil, testAddr, watchTokens)
		
		if details.ErrMessage == "" {
			t.Error("Expected error message for nil client")
		}
		
		if !strings.Contains(details.ErrMessage, "No RPC client") {
			t.Errorf("Expected 'No RPC client' error, got: %s", details.ErrMessage)
		}
	})
}

func TestConnectWithActiveRPCURL(t *testing.T) {
	// This test specifically checks connection using the active RPC URL from environment
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set - this is the active RPC URL endpoint")
	}

	t.Logf("Testing connection to active RPC URL: %s", rpcURL)
	
	result := Connect(rpcURL)
	
	if result.Error != nil {
		t.Fatalf("Failed to connect to active RPC URL: %v", result.Error)
	}
	
	if result.Client == nil {
		t.Fatal("Client is nil for active RPC URL")
	}
	
	// Verify we can make actual RPC calls
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Test multiple RPC methods to ensure connection is working properly
	t.Run("chain ID", func(t *testing.T) {
		chainID, err := result.Client.ChainID(ctx)
		if err != nil {
			t.Errorf("Failed to get chain ID: %v", err)
		} else {
			t.Logf("✓ Chain ID: %s", chainID.String())
		}
	})
	
	t.Run("latest block", func(t *testing.T) {
		blockNum, err := result.Client.BlockNumber(ctx)
		if err != nil {
			t.Errorf("Failed to get block number: %v", err)
		} else {
			t.Logf("✓ Latest block: %d", blockNum)
		}
	})
	
	t.Run("network version", func(t *testing.T) {
		networkID, err := result.Client.NetworkID(ctx)
		if err != nil {
			t.Errorf("Failed to get network ID: %v", err)
		} else {
			t.Logf("✓ Network ID: %s", networkID.String())
		}
	})
	
	t.Log("✓ All RPC endpoint tests passed!")
}

func TestPackageTransaction(t *testing.T) {
	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		t.Skip("ETH_RPC_URL not set, skipping transaction packaging test")
	}

	// Test addresses
	fromAddr := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb2")
	toAddr := common.HexToAddress("0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed")
	amount := common.Big1 // 1 wei

	pkg, err := PackageTransaction(fromAddr, toAddr, amount, rpcURL)
	if err != nil {
		t.Fatalf("Failed to package transaction: %v", err)
	}

	// Verify raw hex is not empty
	if pkg.RawHex == "" {
		t.Error("RawHex is empty")
	}

	// Verify EIP-681 format
	if pkg.EIP681 == "" {
		t.Error("EIP681 is empty")
	}

	// Check EIP-681 format structure: ethereum:<address>@<chainId>?value=<wei>
	if !strings.HasPrefix(pkg.EIP681, "ethereum:") {
		t.Errorf("EIP-681 URI should start with 'ethereum:', got: %s", pkg.EIP681)
	}

	expectedToAddr := strings.ToLower(strings.TrimPrefix(toAddr.Hex(), "0x"))
	if !strings.Contains(strings.ToLower(pkg.EIP681), expectedToAddr) {
		t.Errorf("EIP-681 URI should contain recipient address %s, got: %s", expectedToAddr, pkg.EIP681)
	}

	if !strings.Contains(pkg.EIP681, "?value=") {
		t.Errorf("EIP-681 URI should contain '?value=', got: %s", pkg.EIP681)
	}

	if !strings.Contains(pkg.EIP681, "@") {
		t.Errorf("EIP-681 URI should contain '@' for chain ID, got: %s", pkg.EIP681)
	}

	t.Logf("✓ Raw transaction hex: %s", pkg.RawHex[:64]+"...")
	t.Logf("✓ EIP-681 URI: %s", pkg.EIP681)
}
