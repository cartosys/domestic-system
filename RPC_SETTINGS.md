# RPC Settings Feature

## Overview
The Charm Wallet TUI now includes a built-in settings manager for configuring multiple Ethereum RPC endpoints.

## How to Access
From the main wallet list screen, press `s` to open RPC Settings.

## Features

### List View
- View all configured RPC endpoints
- Active endpoint is marked with a filled circle (●)
- Selected endpoint is highlighted with an arrow (▶)

### Keyboard Controls

**In List Mode:**
- `↑/↓` or `j/k` - Navigate through RPC URLs
- `Enter` or `Space` - Set selected RPC as active (reconnects client)
- `a` - Add new RPC URL
- `e` - Edit selected RPC URL
- `d` or `x` - Delete selected RPC URL
- `Esc` - Return to wallet list

**In Add/Edit Mode:**
- Fill in the form using Charm Huh interface
- `Tab` - Move between fields
- `Enter` - Submit form
- `Esc` - Cancel and return to list

## Configuration Storage
RPC URLs are saved to `~/.charm-wallet-config.json` and persist between sessions.

### Config File Format
```json
{
  "rpc_urls": [
    {
      "Name": "Infura Mainnet",
      "URL": "https://mainnet.infura.io/v3/YOUR_KEY",
      "Active": true
    },
    {
      "Name": "Alchemy Mainnet",
      "URL": "https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY",
      "Active": false
    }
  ]
}
```

## Priority
1. If config file has RPC URLs, uses the active one
2. If no config but `ETH_RPC_URL` environment variable is set, uses that
3. If neither exists, shows "not set" status

## Example Workflow
1. Press `s` from main screen
2. Press `a` to add new RPC
3. Enter name: "My Infura Node"
4. Enter URL: "https://mainnet.infura.io/v3/YOUR_API_KEY"
5. Press `Enter` to save
6. Use `↑/↓` to select your new RPC
7. Press `Enter` to activate it
8. Press `Esc` to return to wallet list

The app will now use your configured RPC endpoint for all blockchain queries!
