package main

import (
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
