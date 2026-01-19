package rpc

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Client wraps an Ethereum RPC client
type Client struct {
	*ethclient.Client
	URL string
}

// ConnectResult holds the result of an RPC connection attempt
type ConnectResult struct {
	Client *Client
	Error  error
}

// Connect attempts to connect to an Ethereum RPC endpoint
func Connect(url string) ConnectResult {
	return ConnectWithTimeout(url, 8*time.Second)
}

// ConnectWithTimeout attempts to connect with a custom timeout
func ConnectWithTimeout(url string, timeout time.Duration) ConnectResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return ConnectResult{Client: nil, Error: err}
	}

	return ConnectResult{
		Client: &Client{
			Client: client,
			URL:    url,
		},
		Error: nil,
	}
}

// TokenBalance represents an ERC20 token balance
type TokenBalance struct {
	Symbol   string
	Decimals uint8
	Balance  *big.Int
}

// WatchedToken represents a token to query
type WatchedToken struct {
	Symbol   string
	Decimals uint8
	Address  common.Address
}

// WalletDetails contains all balance information for a wallet
type WalletDetails struct {
	Address    string
	EthWei     *big.Int
	Tokens     []TokenBalance
	LoadedAt   time.Time
	ErrMessage string
}

// LoadWalletDetails fetches ETH and token balances for an address
func LoadWalletDetails(client *Client, addr common.Address, watch []WatchedToken) WalletDetails {
	return LoadWalletDetailsWithTimeout(client, addr, watch, 12*time.Second)
}

// LoadWalletDetailsWithTimeout fetches wallet details with a custom timeout
func LoadWalletDetailsWithTimeout(client *Client, addr common.Address, watch []WatchedToken, timeout time.Duration) WalletDetails {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	d := WalletDetails{
		Address:  addr.Hex(),
		EthWei:   big.NewInt(0),
		LoadedAt: time.Now(),
	}

	if client == nil || client.Client == nil {
		d.ErrMessage = "No RPC client (set ETH_RPC_URL)."
		return d
	}

	// ETH balance
	wei, err := client.BalanceAt(ctx, addr, nil)
	if err != nil {
		d.ErrMessage = "Failed to load ETH balance."
		return d
	}
	d.EthWei = wei

	// ERC20 balances (simple sequential calls)
	// For speed later: replace with Multicall3 batching.
	var toks []TokenBalance
	for _, t := range watch {
		bal, err := erc20BalanceOf(ctx, client.Client, t.Address, addr)
		if err != nil {
			// skip token silently; you can surface in UI if desired
			continue
		}
		if bal.Sign() > 0 {
			toks = append(toks, TokenBalance{
				Symbol:   t.Symbol,
				Decimals: t.Decimals,
				Balance:  bal,
			})
		}
	}

	sort.Slice(toks, func(i, j int) bool {
		return strings.ToLower(toks[i].Symbol) < strings.ToLower(toks[j].Symbol)
	})
	d.Tokens = toks

	return d
}

// Minimal ERC20 balanceOf via eth_call.
var (
	// balanceOf(address) methodID = keccak256("balanceOf(address)")[:4]
	balanceOfSelector = []byte{0x70, 0xa0, 0x82, 0x31}
)

func erc20BalanceOf(ctx context.Context, client *ethclient.Client, token common.Address, owner common.Address) (*big.Int, error) {
	// calldata = selector + 32-byte left-padded address
	padded := common.LeftPadBytes(owner.Bytes(), 32)
	data := append(balanceOfSelector, padded...)

	// go-ethereum CallContract wants a CallMsg
	msg := ethereum.CallMsg{
		To:   &token,
		Data: data,
	}
	out, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return big.NewInt(0), nil
	}
	return new(big.Int).SetBytes(out), nil
}
