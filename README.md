# The Domestic System

A beautiful, terminal-based Ethereum wallet and smart contract browser built for sovereignty, security, and style.

```
━━━━━━━━━━━━━━━━━━━━
 domestic system
━━━━━━━━━━━━━━━━━━━━
```


![Domestic System Demo](https://lh3.googleusercontent.com/pw/AP1GczPWq0UoFHT5Dy9lw12W3U7MDkK6sz1ZvX_09IVedO_6a8cRcx4wQuHmuKLxIqW3IgbYfOhtPEEiMGlVh1RlFBhqQD5ZNUdkLcHUx9uqF8n2ShhstWR-UQOTzN7pTwU_PIydKTIBj4bk7iMfnNvK2Sgj=w480-h270-s-no?authuser=0)


## Overview

The Domestic System is a terminal user interface (TUI) designed for browsing Ethereum (EVM) accounts and smart contracts. It brings together three core principles that make terminal-based blockchain interaction not just practical, but preferable.

## Philosophy

### 🏛️ Sovereign
**Client-side only. No centralized servers. Just you and the blockchain.**

The Domestic System connects directly to Ethereum RPC endpoints of your choosing. No intermediaries. No analytics. No tracking. Your interaction with the blockchain is direct and unmediated. This is how Web3 was meant to work.

### 🔐 Secure
**No private key management. Hardware wallet ready.**

Your private keys never touch this application. All transactions are generated as QR codes following [EIP-4527](https://eips.ethereum.org/EIPS/eip-4527), designed specifically for signing with hardware wallet devices. View your accounts, prepare transactions, and sign them safely on your hardware wallet.

### ✨ Retro Aesthetic
**Built with Charm.sh. Terminal interfaces, beautifully done.**

Using the gorgeous [Charm.land](https://charm.land) ecosystem (Bubble Tea, Lip Gloss, Huh), The Domestic System proves that CLIs don't have to be austere. Rich colors, smooth interactions, and thoughtful UI design bring a retro-future aesthetic to blockchain interaction.

## Why Terminal?

These three principles—sovereignty, security, and aesthetics—converge perfectly in the terminal. Terminal applications are:
- **Lightweight**: No bloated Electron apps or browser extensions
- **Transparent**: Open source code you can audit
- **Portable**: Works over SSH, on servers, on minimal systems
- **Focused**: No distractions, just the information you need
- **Timeless**: Terminal UIs age gracefully

The Domestic System occupies a unique intersection of niche use cases, serving those who value control, security, and craft.

## Features

- 📊 **Multi-wallet Management**: Track multiple Ethereum addresses simultaneously
- 💰 **Token Balance Viewing**: Native ETH and ERC-20 token support with customizable watchlists
- 🌐 **Multi-RPC Configuration**: Connect to any Ethereum RPC endpoint (Mainnet, L2s, testnets)
- 🔄 **Real-time Updates**: Live balance and network status updates
- 🎨 **Beautiful TUI**: Crafted with Bubble Tea and Lip Gloss for an exceptional terminal experience
- 🦄 **Uniswap Integration**: Swap tokens directly from the terminal
- 📱 **DApp Browser**: Navigate and interact with decentralized applications
- ⚡ **Transaction Building**: Generate transaction QR codes for hardware wallet signing (EIP-4527)
- 📋 **Clipboard Integration**: Easy address copying and ENS resolution
- ⚙️ **Persistent Configuration**: Settings and wallet lists saved locally

## Installation

### Prerequisites

- Go 1.21 or higher
- A terminal with true color support (most modern terminals)

### Build from Source

```bash
git clone https://github.com/yourusername/domestic-system.git
cd domestic-system
go build
go run .
```

## Configuration

The Domestic System stores its configuration in `~/.charm-wallet-config.json`.

### RPC Endpoints

Add Ethereum RPC endpoints in the "s" settings page or by setting the `ETH_RPC_URL` environment variable:

```bash
export ETH_RPC_URL="https://eth.llamarpc.com"
go run .
```

### Token Watchlist

Customize which ERC-20 tokens to display by editing the token watchlist in `main.go` (UI configuration coming soon).

## Usage

### Navigation

- **Arrow Keys / hjkl**: Navigate menus and lists
- **Enter**: Select / Confirm
- **Esc**: Go back / Cancel
- **Tab**: Switch between input fields
- **Ctrl+C**: Quit application
- **Double-click**: Double-click on the active address in the header to open the account list popup and quickly switch between accounts

### Pages

- **Accounts**: Manage your watch-list of Ethereum addresses
- **Details**: View balances and token holdings for the active address
- **Settings**: Configure RPC endpoints and application settings
- **DApps**: Browse and interact with decentralized applications
- **Uniswap**: Token swapping interface

### Adding Accounts

1. Navigate to the Wallets page
2. Press `a` to add a new wallet
3. Enter an Ethereum address or ENS name
4. Optionally provide a nickname
5. Press Enter to save

### Switching Accounts

The active account (marked with ★) determines which wallet is used for transactions. To switch accounts:

1. **Double-click** on the active address shown in the global header (works from any view)
2. Use arrow keys (↑/↓) to select a different account
3. Press Enter to activate the selected account
4. Press Esc to cancel

Alternatively, navigate to the Wallets page and press Enter on any wallet to view its details and activate it.

### Sending Transactions

1. Select a wallet with ETH balance
2. Tab & Click the Send button
3. Fill in recipient address and amount
4. Generate QR code for hardware wallet signing

## Development

## Accessing a remote node via SSH

1. Ensure node is running. By default, the RPC port is often 8545. You can check this by running this on the node machine
```bash
    ss -ntlp | grep 8545 
```
2. On your application machine, create a persistent SSH tunnel to the node machine.
```bash
    ssh -N -L 8545:localhost:8545 user@node_machine_ip_address
```
Your application, running on the app machine, can now access the Ethereum node's RPC via http://localhost:9545


## Uniswap v4 listener as a runnable helper module at uniswap_v4_listener.go. You can now run it from the terminal with:

go run helpers/uniswap_v4_listener.go -ws <wss-url> -poolmanager <address> [-from <block>]

# Listen to new events only
go run helpers/uniswap_v4_listener.go \
  -ws wss://mainnet.infura.io/ws/v3/YOUR_KEY \
  -poolmanager 0x1234567890abcdef1234567890abcdef12345678

# With backfill from a specific block
go run helpers/uniswap_v4_listener.go \
  -ws wss://mainnet.infura.io/ws/v3/YOUR_KEY \
  -poolmanager 0x1234567890abcdef1234567890abcdef12345678 \
  -from 24000000

### Project Structure

```
domestic-system/
├── main.go              # Main TUI application and state management
├── config/              # Configuration loading and saving
├── helpers/             # Utility functions (address formatting, Uniswap, etc.)
├── rpc/                 # Ethereum RPC client wrapper
├── styles/              # Lip Gloss styling definitions
└── views/               # Page-specific rendering logic
    ├── wallets/
    ├── details/
    ├── settings/
    ├── dapps/
    └── uniswap/
```

### Running Tests

```bash
# RPC package tests (requires RPC endpoint)
ETH_RPC_URL="https://eth.llamarpc.com" go test ./rpc -v

# All tests
go test ./... -v
```

## Contributing

Contributions are welcome! This project values:
- **Minimalism**: Add features that align with the core philosophy
- **Security**: No shortcuts with user funds or data
- **Polish**: Terminal UIs deserve attention to detail

Please open an issue before starting major work.

## Roadmap

- [ ] Home dashboard with portfolio overview
- [ ] Transaction history viewing
- [ ] EIP-4527 QR code generation
- [ ] NFT viewing support
- [ ] More DApp integrations
- [ ] Ledger/Trezor direct integration (optional)
- [ ] Mouse support for clicking addresses/buttons

## License

MIT License - See LICENSE file for details

## Acknowledgments

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions for TUIs
- [Huh](https://github.com/charmbracelet/huh) - Terminal forms
- [go-ethereum](https://github.com/ethereum/go-ethereum) - Ethereum client library

Inspired by the ethos of self-sovereignty and the aesthetics of retrocomputing.

---

*The Domestic System: Your terminal, your keys, your blockchain.*
