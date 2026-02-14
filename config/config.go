package config

import (
	"encoding/json"
	"math/big"
	"os"
	"time"
)

// Config represents the application configuration
type Config struct {
	RPCURLs []RPCUrl      `json:"rpc_urls"`
	Wallets []WalletEntry `json:"wallets"`
	Dapps   []DApp        `json:"dapps"`
	Logger  bool          `json:"logger"`
}

// RPCUrl represents an RPC endpoint
type RPCUrl struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// WalletEntry represents a wallet in the config
type WalletEntry struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
	Active  bool   `json:"active"`
}

// DApp represents a dApp in the config
type DApp struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Icon    string `json:"icon,omitempty"`
	Network string `json:"network,omitempty"`
}

// -------------------- UI TYPE DEFINITIONS --------------------

// Page represents a page/view in the application
type Page int

const (
	PageHome Page = iota
	PageWallets
	PageDetails
	PageSettings
	PageDappBrowser
	PageUniswap
)

// ClickableArea represents a clickable region on screen for addresses
type ClickableArea struct {
	X, Y          int    // top-left position
	Width, Height int    // dimensions
	Address       string // wallet address to navigate to
}

// TokenBalance represents an ERC20 token balance
type TokenBalance struct {
	Symbol   string
	Decimals uint8
	Balance  *big.Int
}

// WalletDetails contains all balance information for a wallet
type WalletDetails struct {
	Address    string
	EthWei     *big.Int
	Tokens     []TokenBalance
	LoadedAt   time.Time
	ErrMessage string
}

// -------------------- CONFIG MANAGEMENT --------------------

// Load reads the config from the specified path
func Load(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		// If file doesn't exist, create default config
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			Save(path, cfg)
			return cfg
		}
		return Config{}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}
	}

	return cfg
}

// DefaultConfig returns a new configuration with sensible defaults
func DefaultConfig() Config {
	return Config{
		RPCURLs: []RPCUrl{
			{
				Name:   "Public Mainnet",
				URL:    "https://ethereum-rpc.publicnode.com",
				Active: true,
			},
		},
		Wallets: []WalletEntry{
			{
				Name:    "vitalik.eth",
				Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
				Active:  true,
			},
		},
		Dapps: []DApp{
			{
				Name:    "Uniswap v4",
				Address: "0x000000009B1D0aF20D8C6d0A44e162d11F9b8f00",
				Icon:    "🦄",
				Network: "Mainnet",
			},
		},
		Logger: true,
	}
}

// Save writes the config to the specified path
func Save(path string, cfg Config) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}
