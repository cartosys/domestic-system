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
- 🔑 **Transaction Signer**: Native Go signing module + standalone Python CLI for air-gapped EIP-4527 workflows

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
- **Signer** (`x` from Accounts): Manage signing keys, scan EIP-4527 QR codes via webcam, and sign transactions

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

## Transaction Signer

The Domestic System includes a full EIP-4527 transaction signing workflow usable both inside the TUI and independently from the terminal.

### How it works

```
TUI (any page)  →  packages unsigned tx  →  displays EIP-4527 QR code
                                                        ↓
                                          Signer page scans QR via webcam
                                                        ↓
                                          Decodes UR → signs with stored key
                                                        ↓
                                          Displays raw signed transaction hex
```

### Private key storage

Keys are stored in `~/.charm-wallet-private-keys.json` (mode `600`, outside the repo). On first run a `NotForProduction` demo key is bootstrapped automatically and its address is registered in `~/.charm-wallet-config.json`.

```json
{
  "private_keys": [
    {
      "address": "0x29c73B201bEE86C4d1Fe4f598C4355Bb210251e3",
      "name": "NotForProduction",
      "private_key": "0xc0054..."
    }
  ]
}
```

### TUI Signer page

Press `x` from the Accounts page to open the Signer. From there:

| Key | Action |
|-----|--------|
| `↑`/`↓` | Select signing key |
| `s` | Open webcam — auto-signs the first EIP-4527 QR detected |
| `a` | Reload keys from disk |
| `c` | Clear current result |
| `Esc` | Return to Accounts |

The signed transaction (raw hex, tx hash, r/s/v) is shown immediately after scanning.

### Standalone Python CLI (`signer/eth_signer.py`)

The Python signer runs independently without the TUI. Install dependencies once:

```bash
pip install -r requirements.txt
```

| Command | Description |
|---------|-------------|
| `python signer/eth_signer.py --generate-keys` | Generate a new ECDSA keypair |
| `python signer/eth_signer.py --derive-address 0x...` | Derive address from private key |
| `python signer/eth_signer.py --private-key 0x... --to 0x... --value 1000000000000000000` | Sign a transaction |
| `python signer/eth_signer.py --scan` | Webcam scan: detect and sign EIP-4527 QR codes |
| `python signer/eth_signer.py --add-key --private-key 0x... --name "Wallet"` | Add a key to the store |
| `python signer/eth_signer.py --list-keys` | List stored addresses |

All output is JSON. The `--scan` mode opens an OpenCV window showing the live camera feed and emits signed transactions to stdout as they are detected.

### End-to-end tx signing test CLI (`cmd/txtest`)

Exercises the full pack → decode → sign round-trip using the Go modules directly:

```bash
# Offline (hardcoded test params — no RPC needed)
go run ./cmd/txtest

# With live nonce/gasPrice/chainId from an RPC endpoint
go run ./cmd/txtest --rpc https://eth.llamarpc.com

# Custom recipient and amount
go run ./cmd/txtest --to 0xYourAddr --value 0.5 --chainid 11155111
```

Output shows both steps clearly:

```
STEP 1 — Pack unsigned transaction (EIP-4527)
─────────────────────────────────────────────
Transaction fields: { "from": "0x29c7...", "to": "0xd8dA...", ... }
UR: ur:eth-sign-request/onadtpdagd...

STEP 2 — Decode + sign with stored key
───────────────────────────────────────
Decoded transaction: { "from": "0x29c7...", "value": "0.001000 ETH", ... }
Signed transaction:  { "txHash": "0x5989...", "raw_transaction": "0xf86b...", ... }

✓  Round-trip complete
```

## Development

## Accessing a remote node via SSH

1. Ensure node is running. By default, the RPC port is often 8545. You can check this by running this on the node machine
```bash
    ss -ntlp | grep 8545 
```
2. On your application machine, create a persistent SSH tunnel to the node machine.
```bash
    ssh -N -L 8545:localhost:8545 user@node_machine_ip_address #for http
    ssh -N -L 8546:localhost:8546 user@node_machine_ip_address #for ws websocket 
```
Your application, running on the app machine, can now access the Ethereum node's RPC via http://localhost:8545


## Uniswap v4 listener as a runnable helper module at uniswap_v4_listener.go. You can now run it from the terminal with:

go run helpers/uniswap_v4_listener.go -ws <wss-url> -poolmanager <address> [-from <block>]

# Listen to new events only
go run ./cmd/v4listener \
  -ws ws://localhost:8546 \
  -poolmanager 0x000000000004444c5dc75cB358380D2e3dE08A90

# With backfill from a specific block
go run ./cmd/v4listener \
  -ws ws://localhost:8546 \ #or ws://mainnet.infura.io/ws/v3/YOUR_KEY \
  -poolmanager 0x000000000004444c5dc75cB358380D2e3dE08A90 \
  -from 21688000  

### Project Structure

```
domestic-system/
├── main.go              # Entry point — wires Bubble Tea program
├── model.go             # App state struct + Init()
├── update.go            # All Update() message handlers
├── view.go              # Top-level View() dispatch
├── cmd/
│   ├── txtest/          # End-to-end pack → sign test CLI
│   └── v4listener/      # Standalone Uniswap V4 event listener
├── config/              # JSON config load/save, type definitions
├── helpers/             # ENS resolution, address formatting, Uniswap V2/V4
├── rpc/                 # Ethereum RPC client, EIP-4527 transaction packaging
├── signer/
│   ├── signer.go        # Go package: key management, EIP-4527 decode, ECDSA sign
│   └── eth_signer.py    # Standalone Python CLI (same functionality)
├── styles/              # Lip Gloss colors and shared styles
├── views/               # Per-page renderers
│   ├── wallets/
│   ├── details/
│   ├── settings/
│   ├── dapps/
│   ├── uniswap/
│   ├── terra/
│   └── signer/          # Signer page renderer
└── webcam/              # Camera capture and half-block video rendering
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
- [x] EIP-4527 QR code generation
- [x] EIP-4527 QR code signing (TUI Signer page + Python CLI)
- [ ] NFT viewing support
- [ ] More DApp integrations
- [ ] Ledger/Trezor direct integration (optional)
- [x] Mouse support for clicking addresses/buttons

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
