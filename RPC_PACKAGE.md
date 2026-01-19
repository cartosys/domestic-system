# RPC Package Documentation

## Overview

The RPC package has been successfully extracted from `main.go` into a separate, testable package at `rpc/rpc.go`. This improves code organization and makes the RPC functionality reusable and testable independently.

## Package Structure

```
charm-wallet/
├── main.go              # Main TUI application (now uses rpc package)
├── rpc/
│   ├── rpc.go          # RPC connection and wallet query functions
│   └── rpc_test.go     # Comprehensive test suite
└── go.mod
```

## Exported Types and Functions

### Types

- **`Client`**: Wraps `ethclient.Client` with additional URL tracking
  ```go
  type Client struct {
      *ethclient.Client
      URL string
  }
  ```

- **`ConnectResult`**: Result of RPC connection attempts
  ```go
  type ConnectResult struct {
      Client *Client
      Error  error
  }
  ```

- **`WatchedToken`**: Token configuration for balance queries
  ```go
  type WatchedToken struct {
      Symbol   string
      Decimals uint8
      Address  common.Address
  }
  ```

- **`TokenBalance`**: Token balance result
  ```go
  type TokenBalance struct {
      Symbol   string
      Decimals uint8
      Balance  *big.Int
  }
  ```

- **`WalletDetails`**: Complete wallet balance information
  ```go
  type WalletDetails struct {
      Address    string
      EthWei     *big.Int
      LoadedAt   time.Time
      ErrMessage string
      Tokens     []TokenBalance
  }
  ```

### Functions

- **`Connect(url string) ConnectResult`**  
  Connects to an Ethereum RPC endpoint with default 8-second timeout

- **`ConnectWithTimeout(url string, timeout time.Duration) ConnectResult`**  
  Connects with custom timeout

- **`LoadWalletDetails(client *Client, addr common.Address, watch []WatchedToken) WalletDetails`**  
  Loads ETH and token balances for a wallet (12-second timeout)

- **`LoadWalletDetailsWithTimeout(client *Client, addr common.Address, watch []WatchedToken, timeout time.Duration) WalletDetails`**  
  Loads wallet details with custom timeout

## Tests

The test suite in `rpc/rpc_test.go` includes:

### TestConnect
- Successful connection test (validates chain ID)
- Connection with timeout test
- Invalid URL handling test

### TestLoadWalletDetails
- Load wallet details (tests ETH and token balance queries)
- Nil client handling

### TestConnectWithActiveRPCURL
- Comprehensive RPC endpoint validation:
  - Chain ID verification
  - Latest block number retrieval
  - Network version check

## Running Tests

```bash
# Run all tests with RPC URL from environment
ETH_RPC_URL="https://eth.llamarpc.com" go test ./rpc -v

# Run specific test
ETH_RPC_URL="https://eth.llamarpc.com" go test ./rpc -v -run TestConnect

# Run without environment variable (some tests will be skipped)
go test ./rpc -v
```

## Integration with main.go

The main application now:
1. Imports `charm-wallet-tui/rpc`
2. Uses `*rpc.Client` instead of `*ethclient.Client`
3. Uses `rpc.WatchedToken` for token watchlist
4. Calls `rpc.Connect()` and `rpc.LoadWalletDetails()` instead of inline implementations

### Key Changes

- `model.ethClient`: Changed from `*ethclient.Client` to `*rpc.Client`
- `model.tokenWatch`: Changed from `[]watchedToken` to `[]rpc.WatchedToken`
- `connectRPC()`: Now calls `rpc.Connect(url)`
- `loadDetails()`: Now calls `rpc.LoadWalletDetails()`
- Removed: `erc20BalanceOf()` function (now in rpc package)
- Removed: Local `watchedToken` type definition

### Fixed Variable Shadowing Issue

Fixed a critical bug where a local variable `rpc` was shadowing the imported `rpc` package:
```go
// Before (WRONG - shadows package name):
rpc := strings.TrimSpace(os.Getenv("ETH_RPC_URL"))

// After (CORRECT):
rpcFromEnv := strings.TrimSpace(os.Getenv("ETH_RPC_URL"))
```

## Benefits

1. **Testability**: RPC functionality can now be tested independently
2. **Reusability**: Can be imported by other Go packages if needed
3. **Organization**: Cleaner separation of concerns
4. **Maintainability**: Easier to modify RPC logic without touching UI code
5. **Type Safety**: Exported types provide clear API contracts

## Test Results

Latest test run:
```
=== RUN   TestConnect
=== RUN   TestConnect/successful_connection
    Connected to chain ID: 1
--- PASS: TestConnect (0.13s)

=== RUN   TestLoadWalletDetails
    ETH Balance (wei): 33111613082018244614
    Found 2 token balances:
      USDC: 4130990642
      WETH: 100000000000
--- PASS: TestLoadWalletDetails (4.44s)

=== RUN   TestConnectWithActiveRPCURL
    ✓ Chain ID: 1
    ✓ Latest block: 24270989
    ✓ Network ID: 1
    ✓ All RPC endpoint tests passed!
--- PASS: TestConnectWithActiveRPCURL (0.26s)

PASS
ok      charm-wallet-tui/rpc    4.828s
```

All tests passing successfully!
