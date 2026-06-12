package rpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rlp"
	qrterminal "github.com/mdp/qrterminal/v3"
)

// Client wraps an Ethereum RPC client
type Client struct {
	*ethclient.Client
	URL string
	// DetectedChainID is captured once at connect time (nil if the lookup
	// failed). Named distinctly from the embedded ChainID(ctx) method so that
	// method remains callable on Client.
	DetectedChainID *big.Int
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

	// Best-effort: a failed chain ID lookup leaves ChainID nil, which downstream
	// callers treat as "assume mainnet" (see helpers.UniswapAddressesForChain).
	chainID, _ := client.ChainID(ctx)

	return ConnectResult{
		Client: &Client{
			Client:          client,
			URL:             url,
			DetectedChainID: chainID,
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

// bytewordsLookup maps each byte value (0-255) to its 2-character minimal bytewords encoding.
// Source: Blockchain Commons bc-bytewords specification (https://github.com/BlockchainCommons/bc-bytewords).
// Each pair at [i*2, i*2+1] is the first and last character of the word for byte value i.
var bytewordsLookup = [512]byte{
	'a', 'e', 'a', 'd', 'a', 'o', 'a', 'x', 'a', 'a', 'a', 'h', 'a', 'm', 'a', 't', // 0-7:   able acid also apex aqua arch atom aunt
	'a', 'y', 'a', 's', 'b', 'k', 'b', 'd', 'b', 'n', 'b', 't', 'b', 'a', 'b', 's', // 8-15:  away axis back bald barn belt beta bias
	'b', 'e', 'b', 'y', 'b', 'g', 'b', 'w', 'b', 'b', 'b', 'z', 'c', 'm', 'c', 'h', // 16-23: blue body brag brew bulb buzz calm cash
	'c', 's', 'c', 'f', 'c', 'y', 'c', 'w', 'c', 'e', 'c', 'a', 'c', 'k', 'c', 't', // 24-31: cats chef city claw code cola cook cost
	'c', 'x', 'c', 'l', 'c', 'p', 'c', 'n', 'd', 'k', 'd', 'a', 'd', 's', 'd', 'i', // 32-39: crux curl cusp cyan dark data days deli
	'd', 'e', 'd', 't', 'd', 'r', 'd', 'n', 'd', 'w', 'd', 'p', 'd', 'm', 'd', 'l', // 40-47: dice diet door down draw drop drum dull
	'd', 'y', 'e', 'h', 'e', 'y', 'e', 'o', 'e', 'e', 'e', 'c', 'e', 'n', 'e', 'm', // 48-55: duty each easy echo edge epic even exam
	'e', 't', 'e', 's', 'f', 't', 'f', 'r', 'f', 'n', 'f', 's', 'f', 'm', 'f', 'h', // 56-63: exit eyes fact fair fern figs film fish
	'f', 'z', 'f', 'p', 'f', 'w', 'f', 'x', 'f', 'y', 'f', 'e', 'f', 'g', 'f', 'l', // 64-71: fizz flap flew flux foxy free frog fuel
	'f', 'd', 'g', 'a', 'g', 'e', 'g', 'r', 'g', 's', 'g', 't', 'g', 'l', 'g', 'w', // 72-79: fund gala game gear gems gift girl glow
	'g', 'd', 'g', 'y', 'g', 'm', 'g', 'u', 'g', 'h', 'g', 'o', 'h', 'f', 'h', 'g', // 80-87: good gray grim guru gush gyro half hang
	'h', 'd', 'h', 'k', 'h', 't', 'h', 'p', 'h', 'h', 'h', 'l', 'h', 'y', 'h', 'e', // 88-95: hard hawk heat help high hill holy hope
	'h', 'n', 'h', 's', 'i', 'd', 'i', 'a', 'i', 'e', 'i', 'h', 'i', 'y', 'i', 'o', // 96-103: horn huts iced idea idle inch inky into
	'i', 's', 'i', 'n', 'i', 'm', 'j', 'e', 'j', 'z', 'j', 'n', 'j', 't', 'j', 'l', // 104-111: iris iron item jade jazz join jolt jowl
	'j', 'o', 'j', 's', 'j', 'p', 'j', 'k', 'j', 'y', 'k', 'p', 'k', 'o', 'k', 't', // 112-119: judo jugs jump junk jury keep keno kept
	'k', 's', 'k', 'k', 'k', 'n', 'k', 'g', 'k', 'e', 'k', 'i', 'k', 'b', 'l', 'b', // 120-127: keys kick kiln king kite kiwi knob lamb
	'l', 'a', 'l', 'y', 'l', 'f', 'l', 's', 'l', 'r', 'l', 'p', 'l', 'n', 'l', 't', // 128-135: lava lazy leaf legs liar limp lion list
	'l', 'o', 'l', 'd', 'l', 'e', 'l', 'u', 'l', 'k', 'l', 'g', 'm', 'n', 'm', 'y', // 136-143: logo loud love luau luck lung main many
	'm', 'h', 'm', 'e', 'm', 'o', 'm', 'u', 'm', 'w', 'm', 'd', 'm', 't', 'm', 's', // 144-151: math maze memo menu meow mild mint miss
	'm', 'k', 'n', 'l', 'n', 'y', 'n', 'd', 'n', 's', 'n', 't', 'n', 'n', 'n', 'e', // 152-159: monk nail navy need news next noon note
	'n', 'b', 'o', 'y', 'o', 'e', 'o', 't', 'o', 'x', 'o', 'n', 'o', 'l', 'o', 's', // 160-167: numb obey oboe omit onyx open oval owls
	'p', 'd', 'p', 't', 'p', 'k', 'p', 'y', 'p', 's', 'p', 'm', 'p', 'l', 'p', 'e', // 168-175: paid part peck play plus poem pool pose
	'p', 'f', 'p', 'a', 'p', 'r', 'q', 'd', 'q', 'z', 'r', 'e', 'r', 'p', 'r', 'l', // 176-183: puff puma purr quad quiz race ramp real
	'r', 'o', 'r', 'h', 'r', 'd', 'r', 'k', 'r', 'f', 'r', 'y', 'r', 'n', 'r', 's', // 184-191: redo rich road rock roof ruby ruin runs
	'r', 't', 's', 'e', 's', 'a', 's', 'r', 's', 's', 's', 'k', 's', 'w', 's', 't', // 192-199: rust safe saga scar sets silk skew slot
	's', 'p', 's', 'o', 's', 'g', 's', 'b', 's', 'f', 's', 'n', 't', 'o', 't', 'k', // 200-207: soap solo song stub surf swan taco task
	't', 'i', 't', 't', 't', 'd', 't', 'e', 't', 'y', 't', 'l', 't', 'b', 't', 's', // 208-215: taxi tent tied time tiny toil tomb toys
	't', 'p', 't', 'a', 't', 'n', 'u', 'y', 'u', 'o', 'u', 't', 'u', 'e', 'u', 'r', // 216-223: trip tuna twin ugly undo unit urge user
	'v', 't', 'v', 'y', 'v', 'o', 'v', 'l', 'v', 'e', 'v', 'w', 'v', 'a', 'v', 'd', // 224-231: vast very veto vial vibe view visa void
	'v', 's', 'w', 'l', 'w', 'd', 'w', 'm', 'w', 'p', 'w', 'e', 'w', 'y', 'w', 's', // 232-239: vows wall wand warm wasp wave waxy webs
	'w', 't', 'w', 'n', 'w', 'z', 'w', 'f', 'w', 'k', 'y', 'k', 'y', 'n', 'y', 'l', // 240-247: what when whiz wolf work yank yawn yell
	'y', 'a', 'y', 't', 'z', 's', 'z', 'o', 'z', 't', 'z', 'c', 'z', 'e', 'z', 'm', // 248-255: yoga yurt zaps zero zest zinc zone zoom
}

// encodeBytewordsMinimal encodes bytes using the UR bytewords minimal encoding
// (first + last character of each word, concatenated).
func encodeBytewordsMinimal(data []byte) string {
	result := make([]byte, len(data)*2)
	for i, b := range data {
		result[i*2] = bytewordsLookup[int(b)*2]
		result[i*2+1] = bytewordsLookup[int(b)*2+1]
	}
	return string(result)
}

// cborLengthHeader returns CBOR major-type + length header bytes.
func cborLengthHeader(majorType byte, length int) []byte {
	mt := majorType << 5
	switch {
	case length <= 23:
		return []byte{mt | byte(length)}
	case length <= 0xFF:
		return []byte{mt | 24, byte(length)}
	case length <= 0xFFFF:
		return []byte{mt | 25, byte(length >> 8), byte(length)}
	default:
		return []byte{mt | 26, byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
	}
}

// cborBytesField encodes a byte slice as a CBOR byte string (major type 2).
func cborBytesField(data []byte) []byte {
	h := cborLengthHeader(2, len(data))
	return append(h, data...)
}

// cborUintField encodes a uint64 as a CBOR unsigned integer (major type 0).
func cborUintField(v uint64) []byte {
	switch {
	case v <= 23:
		return []byte{byte(v)}
	case v <= 0xFF:
		return []byte{0x18, byte(v)}
	case v <= 0xFFFF:
		return []byte{0x19, byte(v >> 8), byte(v)}
	default:
		return []byte{0x1A, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
	}
}

// rlpEIP155UnsignedTx is the RLP structure for an EIP-155 unsigned transaction signing preimage.
// Per EIP-155: rlp([nonce, gasPrice, gasLimit, to, value, data, chainId, 0, 0])
type rlpEIP155UnsignedTx struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       common.Address
	Value    *big.Int
	Data     []byte
	V        *big.Int // ChainID (EIP-155 replay protection)
	R        *big.Int // 0 for unsigned
	S        *big.Int // 0 for unsigned
}

// rlpEIP1559UnsignedTx is the RLP structure for an EIP-1559 type-2 signing preimage.
// Per EIP-1559/EIP-2718: 0x02 || rlp([chainId, nonce, maxPriorityFeePerGas, maxFeePerGas, gasLimit, to, value, data, accessList])
type rlpEIP1559UnsignedTx struct {
	ChainID              *big.Int
	Nonce                uint64
	MaxPriorityFeePerGas *big.Int
	MaxFeePerGas         *big.Int
	Gas                  uint64
	To                   common.Address
	Value                *big.Int
	Data                 []byte
	AccessList           []struct{}
}

// buildEthSignRequestCBOR builds the CBOR payload for an EIP-4527 eth-sign-request.
// Map structure:
//
//	1: tag(37, bytes(16)) — request-id UUID
//	2: bytes             — RLP-encoded unsigned transaction
//	3: uint(1)           — data-type = transaction
//	4: uint              — chain-id
//	6: bytes(20)         — from address
func buildEthSignRequestCBOR(requestID [16]byte, signData []byte, chainID uint64, fromAddr common.Address) []byte {
	var buf []byte
	buf = append(buf, 0xA5) // map(5)

	buf = append(buf, 0x01)       // key 1
	buf = append(buf, 0xD8, 0x25) // tag(37) — UUID type
	buf = append(buf, 0x50)       // bytes(16)
	buf = append(buf, requestID[:]...)

	buf = append(buf, 0x02) // key 2
	buf = append(buf, cborBytesField(signData)...)

	buf = append(buf, 0x03) // key 3
	buf = append(buf, 0x01) // uint(1) — transaction type

	buf = append(buf, 0x04) // key 4
	buf = append(buf, cborUintField(chainID)...)

	buf = append(buf, 0x06) // key 6
	buf = append(buf, cborBytesField(fromAddr.Bytes())...)

	return buf
}

// BuildUnsignedTxEIP4527 assembles an EIP-4527 UR from already-known transaction
// parameters. Unlike PackUnsignedTxEIP4527 it does not require an RPC connection,
// making it suitable for offline testing and batch tooling.
func BuildUnsignedTxEIP4527(from, to common.Address, value *big.Int, gasLimit uint64, data []byte, nonce uint64, maxPriorityFeePerGas, maxFeePerGas, chainID *big.Int) (urString string, txJSON string, err error) {
	rlpBytes, err := rlp.EncodeToBytes(&rlpEIP1559UnsignedTx{
		ChainID:              chainID,
		Nonce:                nonce,
		MaxPriorityFeePerGas: maxPriorityFeePerGas,
		MaxFeePerGas:         maxFeePerGas,
		Gas:                  gasLimit,
		To:                   to,
		Value:                value,
		Data:                 data,
		AccessList:           []struct{}{},
	})
	if err != nil {
		return "", "", err
	}
	signData := append([]byte{0x02}, rlpBytes...)

	var requestID [16]byte
	if _, err := rand.Read(requestID[:]); err != nil {
		return "", "", err
	}
	requestID[6] = (requestID[6] & 0x0F) | 0x40
	requestID[8] = (requestID[8] & 0x3F) | 0x80

	cborData := buildEthSignRequestCBOR(requestID, signData, chainID.Uint64(), from)
	checksum := crc32.ChecksumIEEE(cborData)
	payload := append(cborData, byte(checksum>>24), byte(checksum>>16), byte(checksum>>8), byte(checksum))
	urStr := "ur:eth-sign-request/" + encodeBytewordsMinimal(payload)

	txFields := map[string]interface{}{
		"from":                 from.Hex(),
		"to":                   to.Hex(),
		"value":                fmt.Sprintf("0x%x", value),
		"nonce":                fmt.Sprintf("0x%x", nonce),
		"maxPriorityFeePerGas": fmt.Sprintf("0x%x", maxPriorityFeePerGas),
		"maxFeePerGas":         fmt.Sprintf("0x%x", maxFeePerGas),
		"gasLimit":             fmt.Sprintf("0x%x", gasLimit),
		"chainId":              fmt.Sprintf("0x%x", chainID),
		"type":                 "0x2",
	}
	if len(data) > 0 {
		txFields["data"] = "0x" + hex.EncodeToString(data)
	}
	jsonBytes, jsonErr := json.MarshalIndent(txFields, "", "  ")
	if jsonErr != nil {
		txJSON = "{}"
	} else {
		txJSON = string(jsonBytes)
	}
	return urStr, txJSON, nil
}

// PackUnsignedTxEIP4527 fetches live nonce/fees/chainId from rpcURL then
// delegates to BuildUnsignedTxEIP4527.
func PackUnsignedTxEIP4527(from common.Address, to common.Address, value *big.Int, gasLimit uint64, data []byte, rpcURL string) (urString string, txJSON string, err error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return "", "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return "", "", err
	}
	tip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return "", "", err
	}
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return "", "", err
	}
	// baseFee*2 + tip gives headroom for ~5 consecutive max-increase blocks
	maxFee := new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), tip)
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return "", "", err
	}

	return BuildUnsignedTxEIP4527(from, to, value, gasLimit, data, nonce, tip, maxFee, chainID)
}

// TransactionPackageEIP4527 contains transaction data packaged per EIP-4527
type TransactionPackageEIP4527 struct {
	URData  string // UR-encoded CBOR/RLP payload for QR display
	Summary string // Human-readable transaction summary
}

// PackageTransactionEIP4527 creates an EIP-4527 compliant unsigned ETH transfer package.
// The transaction is RLP-encoded, CBOR-wrapped, and UR-encoded for air-gapped signing.
func PackageTransactionEIP4527(fromAddress common.Address, toAddress common.Address, amount *big.Int, rpcURL string) (TransactionPackageEIP4527, error) {
	urStr, _, err := PackUnsignedTxEIP4527(fromAddress, toAddress, amount, 21000, nil, rpcURL)
	if err != nil {
		return TransactionPackageEIP4527{}, err
	}
	summary := "Transfer " + new(big.Float).Quo(
		new(big.Float).SetInt(amount),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
	).Text('f', 6) + " ETH → " + toAddress.Hex()
	return TransactionPackageEIP4527{URData: urStr, Summary: summary}, nil
}

// weiToEthStr formats a wei amount as an ETH string (up to 6 decimal places).
func weiToEthStr(wei *big.Int) string {
	if wei == nil || wei.Sign() == 0 {
		return "0 ETH"
	}
	eth := new(big.Float).Quo(
		new(big.Float).SetInt(wei),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
	)
	return eth.Text('f', 6) + " ETH"
}

// weiToGweiStr formats a wei amount as a Gwei string (up to 4 decimal places).
func weiToGweiStr(wei *big.Int) string {
	if wei == nil || wei.Sign() == 0 {
		return "0 Gwei"
	}
	gwei := new(big.Float).Quo(
		new(big.Float).SetInt(wei),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(9), nil)),
	)
	return gwei.Text('f', 4) + " Gwei"
}

// decodeRawTx hex-decodes and RLP/binary-unmarshals a "0x..."-prefixed signed
// transaction payload.
func decodeRawTx(rawHex string) (*types.Transaction, error) {
	cleaned := strings.TrimPrefix(strings.TrimSpace(rawHex), "0x")
	raw, err := hex.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(raw); err != nil {
		return nil, fmt.Errorf("not a signed transaction: %w", err)
	}
	return tx, nil
}

// DecodedSignedTx holds human-readable fields extracted from a pasted signed
// transaction. The sender is recovered from the transaction's own signature —
// no private key is ever read or required.
type DecodedSignedTx struct {
	Hash          string
	From          string
	To            string
	ValueHuman    string
	Nonce         uint64
	Gas           uint64
	GasPriceHuman string
	ChainID       *big.Int
	JSON          string // pretty-printed transaction JSON
}

// DecodeSignedRawTx parses a "0x..." pre-signed raw transaction and extracts
// display fields for the paste-transaction preview.
func DecodeSignedRawTx(rawHex string) (DecodedSignedTx, error) {
	tx, err := decodeRawTx(rawHex)
	if err != nil {
		return DecodedSignedTx{}, err
	}

	from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return DecodedSignedTx{}, fmt.Errorf("recover sender: %w", err)
	}

	to := ""
	if tx.To() != nil {
		to = tx.To().Hex()
	}

	prettyJSON := ""
	if rawJSON, err := tx.MarshalJSON(); err == nil {
		var buf bytes.Buffer
		if err := json.Indent(&buf, rawJSON, "", "  "); err == nil {
			prettyJSON = buf.String()
		}
	}

	return DecodedSignedTx{
		Hash:          tx.Hash().Hex(),
		From:          from.Hex(),
		To:            to,
		ValueHuman:    weiToEthStr(tx.Value()),
		Nonce:         tx.Nonce(),
		Gas:           tx.Gas(),
		GasPriceHuman: weiToGweiStr(tx.GasPrice()),
		ChainID:       tx.ChainId(),
		JSON:          prettyJSON,
	}, nil
}

// SendRawTransaction decodes a "0x..." pre-signed raw transaction and relays
// it to the connected RPC endpoint via eth_sendRawTransaction. It only
// broadcasts a transaction the user has already signed externally and pasted
// in — the app never signs anything or holds private keys.
func SendRawTransaction(client *Client, rawHex string) (txHash string, err error) {
	if client == nil || client.Client == nil {
		return "", context.DeadlineExceeded
	}

	tx, err := decodeRawTx(rawHex)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if err := client.SendTransaction(ctx, tx); err != nil {
		return "", err
	}
	return tx.Hash().Hex(), nil
}

// TxOnChainInfo holds human-readable on-chain data for a confirmed (mined) transaction.
type TxOnChainInfo struct {
	Hash              string
	Status            string // "Success" or "Failed"
	BlockNumber       uint64
	BlockHash         string
	From              string
	To                string
	ValueHuman        string
	Nonce             uint64
	GasUsed           uint64
	EffectiveGasPrice string
	TransactionIndex  uint
	Confirmations     uint64
}

// GetTransactionOnChain looks up a broadcast transaction by hash and returns
// its on-chain data once mined. (nil, false, nil) means the transaction
// hasn't been mined yet — the expected state while polling, not an error.
func GetTransactionOnChain(client *Client, txHash common.Hash) (*TxOnChainInfo, bool, error) {
	if client == nil || client.Client == nil {
		return nil, false, context.DeadlineExceeded
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	receipt, err := client.TransactionReceipt(ctx, txHash)
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	tx, _, err := client.TransactionByHash(ctx, txHash)
	if err != nil {
		return nil, false, err
	}

	status := "Failed"
	if receipt.Status == types.ReceiptStatusSuccessful {
		status = "Success"
	}

	to := ""
	if tx.To() != nil {
		to = tx.To().Hex()
	}
	fromStr := ""
	if from, sErr := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx); sErr == nil {
		fromStr = from.Hex()
	}

	var confirmations uint64
	if head, hErr := GetBlockHeight(client); hErr == nil && head >= receipt.BlockNumber.Uint64() {
		confirmations = head - receipt.BlockNumber.Uint64() + 1
	}

	return &TxOnChainInfo{
		Hash:              receipt.TxHash.Hex(),
		Status:            status,
		BlockNumber:       receipt.BlockNumber.Uint64(),
		BlockHash:         receipt.BlockHash.Hex(),
		From:              fromStr,
		To:                to,
		ValueHuman:        weiToEthStr(tx.Value()),
		Nonce:             tx.Nonce(),
		GasUsed:           receipt.GasUsed,
		EffectiveGasPrice: weiToGweiStr(receipt.EffectiveGasPrice),
		TransactionIndex:  receipt.TransactionIndex,
		Confirmations:     confirmations,
	}, true, nil
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

// generateQRCodeCompact renders a QR code using half-block Unicode characters so
// each module occupies one terminal column (half the width of the full-block mode).
func generateQRCodeCompact(data string) string {
	var buf bytes.Buffer
	config := qrterminal.Config{
		HalfBlocks:     true,
		Level:          qrterminal.L,
		Writer:         &buf,
		BlackChar:      qrterminal.BLACK_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteChar:      qrterminal.WHITE_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone:      1,
	}
	qrterminal.GenerateWithConfig(data, config)
	return buf.String()
}

// decodeBytewordsMinimal is the inverse of encodeBytewordsMinimal.
func decodeBytewordsMinimal(encoded string) ([]byte, error) {
	if len(encoded)%2 != 0 {
		return nil, fmt.Errorf("odd bytewords length %d", len(encoded))
	}
	type pair [2]byte
	rev := make(map[pair]byte, 256)
	for i := 0; i < 256; i++ {
		rev[pair{bytewordsLookup[i*2], bytewordsLookup[i*2+1]}] = byte(i)
	}
	out := make([]byte, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		b, ok := rev[pair{encoded[i], encoded[i+1]}]
		if !ok {
			return nil, fmt.Errorf("invalid bytewords pair at %d: %q", i, encoded[i:i+2])
		}
		out[i/2] = b
	}
	return out, nil
}

// GenerateAnimatedQRFrames splits a single-part UR string into BCUR-style
// sequential multi-part frames and returns compact half-block QR ASCII art for
// each frame.  maxChunkBytes controls how many bytes of the original CBOR
// payload go into each part; smaller values produce more frames but each QR
// fits on screen without horizontal scrolling.
func GenerateAnimatedQRFrames(urString string, maxChunkBytes int) ([]string, error) {
	// Parse "ur:TYPE/BYTEWORDS"
	if !strings.HasPrefix(urString, "ur:") {
		return nil, fmt.Errorf("not a UR string")
	}
	rest := urString[3:]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return nil, fmt.Errorf("missing slash in UR")
	}
	urType := rest[:slash]
	bwStr := rest[slash+1:]

	// Decode bytewords → cborData + 4-byte CRC32 suffix
	payload, err := decodeBytewordsMinimal(bwStr)
	if err != nil {
		return nil, fmt.Errorf("bytewords decode: %w", err)
	}
	if len(payload) < 5 {
		return nil, fmt.Errorf("payload too short")
	}
	cborData := payload[:len(payload)-4]
	storedCRC := uint32(payload[len(payload)-4])<<24 |
		uint32(payload[len(payload)-3])<<16 |
		uint32(payload[len(payload)-2])<<8 |
		uint32(payload[len(payload)-1])
	if crc32.ChecksumIEEE(cborData) != storedCRC {
		return nil, fmt.Errorf("CRC32 mismatch in source UR")
	}

	msgLen := len(cborData)
	msgCRC := crc32.ChecksumIEEE(cborData)
	numChunks := (msgLen + maxChunkBytes - 1) / maxChunkBytes
	if numChunks < 1 {
		numChunks = 1
	}

	frames := make([]string, numChunks)
	for i := 0; i < numChunks; i++ {
		start := i * maxChunkBytes
		end := start + maxChunkBytes
		if end > msgLen {
			end = msgLen
		}
		chunk := cborData[start:end]

		// BCUR multi-part part: CBOR array(5)[seqNum, seqLen, msgLen, msgCRC, fragment]
		var part []byte
		part = append(part, 0x85) // array(5)
		part = append(part, cborUintField(uint64(i+1))...)
		part = append(part, cborUintField(uint64(numChunks))...)
		part = append(part, cborUintField(uint64(msgLen))...)
		part = append(part, cborUintField(uint64(msgCRC))...)
		part = append(part, cborBytesField(chunk)...)

		// Append CRC32 of this part (mirrors the single-part payload convention)
		pc := crc32.ChecksumIEEE(part)
		part = append(part, byte(pc>>24), byte(pc>>16), byte(pc>>8), byte(pc))

		frameUR := fmt.Sprintf("ur:%s/%d-%d/%s", urType, i+1, numChunks, encodeBytewordsMinimal(part))
		frames[i] = generateQRCodeCompact(frameUR)
	}
	return frames, nil
}