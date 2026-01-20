# AGENTS.md

## Overview
Go module `charm-wallet-tui` implementing a Bubble Tea TUI wallet browser in `main.go`, with RPC functionality split into the `rpc/` package.

## Essential Commands (observed)
- Run RPC package tests (requires RPC URL env var for full coverage):
  ```bash
  ETH_RPC_URL="https://eth.llamarpc.com" go test ./rpc -v
  ```
- Run a specific RPC test:
  ```bash
  ETH_RPC_URL="https://eth.llamarpc.com" go test ./rpc -v -run TestConnect
  ```
- Run RPC tests without an env var (some tests skip):
  ```bash
  go test ./rpc -v
  ```

## Project Structure
- `main.go`: Bubble Tea TUI application, UI state machine, rendering, config loading/saving.
- `rpc/rpc.go`: RPC client wrapper, wallet balance loading, ERC-20 balance calls.
- `rpc/rpc_test.go`: RPC integration tests (skip without `ETH_RPC_URL`).
- `RPC_PACKAGE.md`, `RPC_SETTINGS.md`: Feature and package documentation.

## Configuration & Environment
- Config file: `~/.charm-wallet-config.json` (stores `rpc_urls` and `wallets`).
- Environment variable: `ETH_RPC_URL` is used if no RPC URLs are configured.

## Code Patterns & Conventions
- Bubble Tea model update/view pattern in `main.go` (single `model` struct and `Update`/`View` methods).
- RPC package exposes `Client`, `Connect`, and `LoadWalletDetails` APIs used by the TUI.
- Token watchlist is defined in `main.go` as a slice of `rpc.WatchedToken`.

## Testing Notes
- Tests in `rpc/rpc_test.go` require a reachable Ethereum RPC endpoint; they skip if `ETH_RPC_URL` is unset.
- Tests perform live network calls (chain ID, latest block, token balances).