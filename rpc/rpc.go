package rpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	qrterminal "github.com/mdp/qrterminal/v3"
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
		d.ErrMessage = "No RPC client connected. Configure an RPC endpoint in Settings."
		return d
	}

	// ETH balance
	wei, err := client.BalanceAt(ctx, addr, nil)
	if err != nil {
		d.ErrMessage = "Failed to load ETH balance: " + err.Error()
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

// GetBlockHeight retrieves the latest block number from the RPC endpoint
func GetBlockHeight(client *Client) (uint64, error) {
	return GetBlockHeightWithTimeout(client, 5*time.Second)
}

// GetBlockHeightWithTimeout retrieves the latest block number with a custom timeout
func GetBlockHeightWithTimeout(client *Client, timeout time.Duration) (uint64, error) {
	if client == nil || client.Client == nil {
		return 0, context.DeadlineExceeded
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	blockNumber, err := client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}

	return blockNumber, nil
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

// TransactionPackage contains both raw hex and EIP-681 formatted transaction data
type TransactionPackage struct {
	RawHex string // RLP-encoded transaction hex
	EIP681 string // EIP-681 formatted URI (ethereum:<address>@<chainId>?value=<wei>)
}

// PackageTransaction creates an unsigned transaction with both raw hex and EIP-681 format.
func PackageTransaction(fromAddress common.Address, toAddress common.Address, amount *big.Int, rpcURL string) (TransactionPackage, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return TransactionPackage{}, err
	}

	// 1. Fetch current network requirements
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return TransactionPackage{}, err
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return TransactionPackage{}, err
	}

	// Get chain ID
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return TransactionPackage{}, err
	}

	// 2. Create the transaction object
	gasLimit := uint64(21000) 
	tx := types.NewTransaction(nonce, toAddress, amount, gasLimit, gasPrice, nil)

	// 3. Serialize the transaction using RLP encoding
	// MarshalBinary returns the RLP-encoded bytes of the transaction
	rawTxBytes, err := tx.MarshalBinary()
	if err != nil {
		return TransactionPackage{}, err
	}

	// 4. Format as EIP-681 URI: ethereum:<address>@<chainId>?value=<wei>
	// Use checksummed address (with capital letters) as per EIP-55
	eip681URI := "ethereum:" + toAddress.Hex() + "@" + chainID.String() + "?value=" + amount.String()

	// 5. Return both formats
	return TransactionPackage{
		RawHex: hex.EncodeToString(rawTxBytes),
		EIP681: eip681URI,
	}, nil
}

// TransactionPackageEIP4527 contains transaction data in EIP-4527 format
type TransactionPackageEIP4527 struct {
	JSON   string // JSON-formatted transaction per EIP-4527
	QRData string // The data to encode in QR code (JSON string)
}

// PackageTransactionEIP4527 creates a transaction package using EIP-4527 format.
// EIP-4527 defines a JSON schema for encoding Ethereum transactions in QR codes.
func PackageTransactionEIP4527(fromAddress common.Address, toAddress common.Address, amount *big.Int, rpcURL string) (TransactionPackageEIP4527, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return TransactionPackageEIP4527{}, err
	}

	// 1. Fetch current network requirements
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return TransactionPackageEIP4527{}, err
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return TransactionPackageEIP4527{}, err
	}

	// Get chain ID
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return TransactionPackageEIP4527{}, err
	}

	gasLimit := uint64(21000)

	// 2. Create EIP-4527 formatted JSON with minimal encoding
	// Use shortest possible hex encoding (no leading zeros)
	// EIP-4527 uses a JSON schema with fields: from, to, value, gas, gasPrice, nonce, chainId
	eip4527JSON := `{` +
		`"from":"` + fromAddress.Hex() + `",` +
		`"to":"` + toAddress.Hex() + `",` +
		`"value":"0x` + amount.Text(16) + `",` +
		`"gas":"0x` + new(big.Int).SetUint64(gasLimit).Text(16) + `",` +
		`"gasPrice":"0x` + gasPrice.Text(16) + `",` +
		`"nonce":"0x` + new(big.Int).SetUint64(nonce).Text(16) + `",` +
		`"chainId":"0x` + chainID.Text(16) + `"` +
		`}`
	
	// 3. Create a compact version for QR code (remove optional fields and whitespace)
	// Minimal version: just to, value, and chainId (enough for basic transaction)
	compactQR := `{` +
		`"to":"` + toAddress.Hex() + `",` +
		`"value":"0x` + amount.Text(16) + `",` +
		`"chainId":"0x` + chainID.Text(16) + `"` +
		`}`

	// 4. Return the package with full JSON for display and compact version for QR
	return TransactionPackageEIP4527{
		JSON:   eip4527JSON,
		QRData: compactQR,
	}, nil
}

// GenerateQRCode converts a string into a QR code representation for terminal display.
// Returns the QR code as a string that can be rendered in the TUI.
func GenerateQRCode(data string) string {
	var buf bytes.Buffer
	config := qrterminal.Config{
		Level:     qrterminal.L,
		Writer:    &buf,
		BlackChar: qrterminal.BLACK,
		WhiteChar: qrterminal.WHITE,
		QuietZone: 1,
	}
	qrterminal.GenerateWithConfig(data, config)
	return buf.String()
}