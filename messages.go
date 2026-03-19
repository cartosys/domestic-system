package main

import (
	"math/big"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
)

// -------------------- TEA MESSAGES --------------------
// All custom message types for The Elm Architecture

// clipboardCopiedMsg indicates clipboard copy completed
type clipboardCopiedMsg struct{}

// txJsonCopiedMsg indicates transaction JSON was copied to clipboard
type txJsonCopiedMsg struct{}

// ensLookupResultMsg contains result of reverse ENS lookup (address -> name)
type ensLookupResultMsg struct {
	address   string
	ensName   string
	err       error
	debugInfo string
}

// ensForwardResolveMsg contains result of forward ENS resolution (name -> address)
type ensForwardResolveMsg struct {
	ensName   string // The .eth name that was resolved
	address   string // The resolved Ethereum address
	err       error
	debugInfo string
}

// uniswapQuoteMsg contains result of Uniswap price quote fetch
type uniswapQuoteMsg struct {
	quote *helpers.SwapQuote
	err   error
}

// logInitMsg signals that log viewport should be initialized
type logInitMsg struct{}

// rpcConnectedMsg contains result of RPC connection attempt
type rpcConnectedMsg struct {
	client *rpc.Client
	err    error
}

// detailsLoadedMsg contains wallet balance details after loading
type detailsLoadedMsg struct {
	d   config.WalletDetails
	err error
}

// packageTransactionMsg contains packaged transaction ready for QR display
type packageTransactionMsg struct {
	txDisplay string
	qrData    string
	format    string
	err       error
}

// terraNullClaimsCountMsg contains the result of a number_of_claims() call
type terraNullClaimsCountMsg struct {
	count *big.Int
	err   error
}

// terraNullClaimQueryMsg contains the result of a claims(uint256) call
type terraNullClaimQueryMsg struct {
	result *helpers.TerraClaimResult
	err    error
}

// poolEventLineMsg carries a single formatted pool event line for the log panel
type poolEventLineMsg struct {
	line string
}

// poolEventMonitorStoppedMsg signals that the pool event monitor has stopped
type poolEventMonitorStoppedMsg struct{}

// poolInfoResultMsg carries the result of a FetchPoolInfo call
type poolInfoResultMsg struct {
	poolID string
	info   *helpers.PoolInfo
	err    error
}

// poolKeyResultMsg carries the result of a FetchPoolKey call
type poolKeyResultMsg struct {
	poolID string
	key    *helpers.PoolKeyInfo
	err    error
}

// liquidityPositionsMsg carries the result of a V4 liquidity position lookup
type liquidityPositionsMsg struct {
	positions   []helpers.LiquidityPosition
	nftCount    uint64    // total NFTs reported by balanceOf before filtering
	diagnostics []string  // per-step diagnostic lines for the logger
	err         error
}
