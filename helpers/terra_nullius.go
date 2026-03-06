package helpers

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// TerraContractAddress is the deployed Terra Nullius contract on Ethereum mainnet
const TerraContractAddress = "0x6e38A457C722C6011B2DfA06d49240e797844d66"

// Terra Nullius ABI function selectors (keccak256 of signature, first 4 bytes)
var (
	terraNullClaimsCountSelector = crypto.Keccak256([]byte("number_of_claims()"))[:4]
	terraNullClaimsSelector      = crypto.Keccak256([]byte("claims(uint256)"))[:4]
	TerraClaimSelector           = crypto.Keccak256([]byte("claim(string)"))[:4]
)

// TerraClaimResult holds data returned by the claims(uint256) function
type TerraClaimResult struct {
	Claimant    string
	Message     string
	BlockNumber *big.Int
}

// GetTerraNumberOfClaims calls number_of_claims() on the Terra Nullius contract
func GetTerraNumberOfClaims(client *ethclient.Client) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	addr := common.HexToAddress(TerraContractAddress)
	callMsg := ethereum.CallMsg{
		To:   &addr,
		Data: terraNullClaimsCountSelector,
	}
	result, err := client.CallContract(ctx, callMsg, nil)
	if err != nil {
		return nil, fmt.Errorf("number_of_claims: %w", err)
	}
	if len(result) < 32 {
		return nil, fmt.Errorf("number_of_claims: short response (%d bytes)", len(result))
	}
	return new(big.Int).SetBytes(result[0:32]), nil
}

// GetTerraClaim calls claims(uint256) on the Terra Nullius contract.
// Returns claimant address, message string, and block number.
func GetTerraClaim(client *ethclient.Client, index *big.Int) (*TerraClaimResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	addr := common.HexToAddress(TerraContractAddress)

	// ABI-encode the uint256 argument: 32 bytes, big-endian
	indexBytes := make([]byte, 32)
	index.FillBytes(indexBytes)

	data := make([]byte, 4+32)
	copy(data[0:4], terraNullClaimsSelector)
	copy(data[4:36], indexBytes)

	callMsg := ethereum.CallMsg{
		To:   &addr,
		Data: data,
	}
	result, err := client.CallContract(ctx, callMsg, nil)
	if err != nil {
		return nil, fmt.Errorf("claims: %w", err)
	}
	if len(result) < 96 {
		return nil, fmt.Errorf("claims: short response (%d bytes)", len(result))
	}

	// Decode ABI-encoded (address, string, uint256):
	// [0:32]  = address (zero-padded, last 20 bytes are the address)
	var claimantAddr common.Address
	copy(claimantAddr[:], result[12:32])

	// [32:64] = offset pointer to string data (relative to start of response)
	strOffset := new(big.Int).SetBytes(result[32:64]).Uint64()

	// [64:96] = block_number
	blockNumber := new(big.Int).SetBytes(result[64:96])

	// At strOffset: 32-byte string length, followed by string bytes
	if uint64(len(result)) < strOffset+32 {
		return nil, fmt.Errorf("claims: response too short for string at offset %d", strOffset)
	}
	strLen := new(big.Int).SetBytes(result[strOffset : strOffset+32]).Uint64()

	var message string
	if strLen > 0 {
		strStart := strOffset + 32
		strEnd := strStart + strLen
		if uint64(len(result)) >= strEnd {
			message = string(result[strStart:strEnd])
		} else {
			message = string(result[strStart:])
		}
	}

	return &TerraClaimResult{
		Claimant:    claimantAddr.Hex(),
		Message:     message,
		BlockNumber: blockNumber,
	}, nil
}

// BuildTerraClaimCalldata builds ABI-encoded calldata for claim(string message).
// Layout: [selector(4)] [offset(32)] [length(32)] [data(padded to 32-byte boundary)]
func BuildTerraClaimCalldata(message string) []byte {
	msgBytes := []byte(message)
	msgLen := len(msgBytes)
	padded := ((msgLen + 31) / 32) * 32

	buf := make([]byte, 4+32+32+padded)
	copy(buf[0:4], TerraClaimSelector)

	// ABI offset points to start of string encoding = 32 bytes past offset slot
	buf[35] = 0x20 // offset = 32

	// String length
	lenBig := new(big.Int).SetUint64(uint64(msgLen))
	lenBytes := make([]byte, 32)
	lenBig.FillBytes(lenBytes)
	copy(buf[36:68], lenBytes)

	// String data (zero-padded to 32-byte boundary)
	copy(buf[68:68+msgLen], msgBytes)
	return buf
}
