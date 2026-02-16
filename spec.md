# Domestic System — Software Specification

## Summary
Domestic System is a terminal-based Ethereum wallet and smart-contract browser built on the Charm.sh TUI stack. It provides multi-wallet balance viewing, multi-RPC configuration, Uniswap swap quoting, dApp browsing, and transaction packaging for hardware wallet signing (EIP-681 and EIP-4527 QR output). All interactions are client-side and connect directly to user-selected RPC endpoints.

## Vision
Deliver a sovereign, secure, and beautifully designed terminal interface for Ethereum accounts and dApps that prioritizes privacy, reliability, and clarity.

## Goals
- Provide a fast, keyboard- and mouse-friendly TUI for managing multiple Ethereum addresses.
- Offer reliable balance and token data via direct RPC connections.
- Enable safe transaction packaging without key custody (QR-based signing).
- Include built-in settings for RPC endpoints and dApp entries.
- Provide Uniswap V2 swap quoting inside the TUI.
- Maintain a cohesive retro-future UI aesthetic.

## Non-Goals
- No private key storage or signing within the app.
- No custodial services or hosted backend.
- No on-chain transaction broadcasting within the TUI.
- No advanced trading features (routing, gas estimation, MEV protection).

## Target Users
- Privacy-focused Ethereum users who prefer local tools.
- Hardware wallet users who want QR-based signing flows.
- Developers and power users who want RPC-configurable wallet browsing.

## Core User Stories
- As a user, I can add and manage multiple wallets and set an active wallet.
- As a user, I can view ETH and ERC-20 balances for the active wallet.
- As a user, I can configure multiple RPC endpoints and switch between them.
- As a user, I can add and browse dApps and open the Uniswap swap view.
- As a user, I can generate QR-ready transaction payloads without exposing keys.
- As a user, I can resolve ENS names to addresses and vice versa.

## Functional Requirements

### Wallet Management
- Add wallets by address or ENS name.
- Prevent duplicate wallet entries.
- Support optional wallet nicknames.
- Allow switching active wallet via list selection or header double-click popup.
- Persist wallets to local config.

### Wallet Details
- Display ETH balance and token balances for a watchlist.
- Cache wallet details per address to reduce redundant RPC calls.
- Allow manual refresh.
- Provide clipboard copy for addresses.

### RPC Settings
- List, add, edit, and delete RPC endpoints.
- Mark one endpoint as active.
- Fall back to `ETH_RPC_URL` when no config entries exist.
- Persist RPC settings to local config.

### dApp Browser
- List, add, and edit dApp entries (name, address, icon, network).
- Use RPC entries for network selection.
- Launch Uniswap view from selected dApp.

### Uniswap Swap View
- Support USDC ↔ ETH (WETH) quoting via Uniswap V2 pair reserves.
- Forward quote: compute output amount from input.
- Reverse quote: compute required input from target output.
- Display price impact warnings for moderate/high slippage.
- Log quote details to the log panel.

### Transaction Packaging
- ETH transfer: generate EIP-681 URI and QR output.
- Swap: generate EIP-4527 JSON payload and QR output (informational for router call).
- Never store or sign transactions locally.

### ENS Resolution
- Resolve `.eth` names to addresses (forward lookup).
- Resolve addresses to ENS names (reverse lookup).
- Populate nickname field on successful resolution.

### Logging Panel
- Toggleable log panel for diagnostics and swap quote details.
- Log levels: debug, info, warn, error.
- Persistent log buffer during session.

### Input & Navigation
- Keyboard navigation (arrows, hjkl, tab, enter, esc).
- Mouse support for clickable addresses and double-click activation.
- Clipboard shortcuts for copy actions.

## Data & Configuration

### Config File
- Location: `~/.charm-wallet-config.json`
- Schema:
  ```json
  {
    "rpc_urls": [{ "name": "...", "url": "...", "active": true }],
    "wallets": [{ "address": "...", "name": "...", "active": true }],
    "dapps": [{ "name": "...", "address": "...", "icon": "...", "network": "..." }],
    "logger": true
  }
  ```

### Runtime State
- Active page: wallets, details, settings, dapps, uniswap.
- Cached wallet details by address.
- Uniswap quote state, editing state, and price impact warnings.

## Architecture
- **UI Layer**: Bubble Tea model/update/view pattern.
- **Views**: Modular view rendering in `views/`.
- **RPC Layer**: `rpc/` package encapsulating connections, balances, and QR generation.
- **Helpers**: ENS resolution, formatting, Uniswap quote logic.
- **Config**: JSON-based persistence in `config/`.

## External Dependencies
- Charm.sh: Bubble Tea, Lip Gloss, Huh.
- go-ethereum for RPC and on-chain calls.
- go-ens for ENS resolution.
- qrterminal for QR display.

## Security & Privacy
- No private key handling in-app.
- No telemetry or external analytics.
- RPC connections are direct to user-configured endpoints.
- Transaction payloads are generated locally and displayed as QR.

## Reliability & Performance
- RPC timeouts for connect (8s) and balance loads (12s).
- Cached wallet details to reduce repeat RPC calls.
- Quote computations use direct contract calls with bounded timeouts.

## Error Handling
- Surface RPC connection failures in UI and logs.
- Handle missing or invalid addresses and ENS failures gracefully.
- Skip token balances if ERC-20 calls fail.

## Test Plan
- Unit/integration tests in `rpc/` and `helpers/`.
- Run all tests:
  ```bash
  go test ./... -v
  ```
- RPC integration tests may skip unless `ETH_RPC_URL` is set.

## Roadmap (from README)
- Home dashboard with portfolio overview.
- Transaction history viewing.
- EIP-4527 QR code generation (fully standardized output).
- NFT viewing support.
- More dApp integrations.
- Ledger/Trezor direct integration.
- Expanded mouse support.

## Known Constraints
- Swap quoting currently supports USDC ↔ ETH only.
- Swap execution is packaging-only; no on-chain broadcast.
- Requires a reachable Ethereum RPC endpoint for live data.
