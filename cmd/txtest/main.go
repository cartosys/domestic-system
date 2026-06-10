// cmd/txtest — two-step EIP-4527 transaction test using domestic system modules.
//
// Step 1 (pack):  builds an unsigned EIP-4527 UR using rpc.BuildUnsignedTxEIP4527
//                 (or rpc.PackUnsignedTxEIP4527 when --rpc is supplied).
// Step 2 (sign):  decodes the UR and signs it using signer.DecodeEIP4527UR +
//                 signer.SignTx — exactly the same code path the TUI uses.
//
// Usage:
//   go run ./cmd/txtest [flags]
//
// Flags:
//   --from    sender address   (default: NotForProduction wallet)
//   --key     private key hex  (default: NotForProduction key)
//   --to      recipient        (default: vitalik.eth address)
//   --value   amount in ETH    (default: 0.0005)
//   --nonce   tx nonce         (default: 0)
//   --gasprice gwei            (default: 20)
//   --gaslimit                 (default: 21000)
//   --chainid                  (default: 1)
//   --rpc     RPC URL          (optional — fetches live nonce/gasPrice/chainId)
//             falls back to ETH_RPC_URL env var
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"

	"charm-wallet-tui/rpc"
	"charm-wallet-tui/signer"

	"github.com/ethereum/go-ethereum/common"
)

func main() {
	fromFlag     := flag.String("from",     signer.DefaultAddress(),     "sender address")
	keyFlag      := flag.String("key",      signer.DefaultPrivateKey,    "private key (hex)")
	toFlag       := flag.String("to",       "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", "recipient address")
	valueFlag    := flag.Float64("value",   0.0005, "amount in ETH")
	nonceFlag    := flag.Uint64("nonce",    0,       "transaction nonce")
	gasPriceFlag := flag.Uint64("gasprice", 20,     "gas price in Gwei")
	gasLimitFlag := flag.Uint64("gaslimit", 21000,  "gas limit")
	chainIDFlag  := flag.Int64("chainid",  1,       "chain ID (1=mainnet)")
	rpcFlag      := flag.String("rpc",     "",      "RPC URL (fetches live params; overrides nonce/gasprice/chainid)")
	flag.Parse()

	// Resolve ETH_RPC_URL fallback
	rpcURL := *rpcFlag
	if rpcURL == "" {
		rpcURL = os.Getenv("ETH_RPC_URL")
	}

	from    := common.HexToAddress(*fromFlag)
	to      := common.HexToAddress(*toFlag)
	chainID := big.NewInt(*chainIDFlag)

	// Convert ETH → wei
	weiPerEth := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	valueWeiF := new(big.Float).Mul(big.NewFloat(*valueFlag), weiPerEth)
	valueWei, _ := valueWeiF.Int(nil)

	printSection("STEP 1 — Pack unsigned transaction (EIP-4527)")

	var urStr, txJSON string
	var err error

	if rpcURL != "" {
		fmt.Fprintf(os.Stderr, "  Using live RPC: %s\n", rpcURL)
		fmt.Fprintf(os.Stderr, "  (fetching nonce / gas price / chain ID …)\n\n")
		urStr, txJSON, err = rpc.PackUnsignedTxEIP4527(from, to, valueWei, *gasLimitFlag, nil, rpcURL)
	} else {
		fmt.Fprintf(os.Stderr, "  No RPC URL — using provided/default params (nonce=%d, maxFeePerGas=%d Gwei, chainId=%d)\n\n",
			*nonceFlag, *gasPriceFlag, *chainIDFlag)
		maxFeeWei := new(big.Int).Mul(big.NewInt(int64(*gasPriceFlag)), big.NewInt(1_000_000_000))
		urStr, txJSON, err = rpc.BuildUnsignedTxEIP4527(from, to, valueWei, *gasLimitFlag, nil,
			*nonceFlag, maxFeeWei, maxFeeWei, chainID)
	}
	fatal(err, "pack")

	printJSON("Transaction fields", txJSON)
	fmt.Printf("UR:  %s\n\n", urStr)

	printSection("STEP 2 — Decode + sign with stored key")

	decoded, decErr := signer.DecodeEIP4527UR(urStr)
	fatal(decErr, "decode")

	printJSON("Decoded transaction", mustJSON(map[string]interface{}{
		"from":     decoded.From,
		"to":       decoded.To,
		"value":    signer.WeiToEthStr(decoded.Value),
		"nonce":    decoded.Nonce,
		"gasPrice": decoded.GasPrice.String() + " wei",
		"gas":      decoded.Gas,
		"chainId":  decoded.ChainID.String(),
	}))

	result, signErr := signer.SignTx(decoded, *keyFlag)
	fatal(signErr, "sign")

	printJSON("Signed transaction", mustJSON(map[string]interface{}{
		"from":            result.From,
		"to":              result.To,
		"value":           result.ValueHuman,
		"txHash":          result.TxHash,
		"r":               result.R,
		"s":               result.S,
		"v":               result.V,
		"raw_transaction": result.RawTx,
	}))

	fmt.Println("✓  Round-trip complete — unsigned UR encoded, decoded, and signed successfully.")
}

func printSection(title string) {
	fmt.Printf("\n%s\n%s\n", title, strings.Repeat("─", len(title)))
}

func printJSON(label, jsonStr string) {
	fmt.Printf("%s:\n%s\n\n", label, jsonStr)
}

func mustJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func fatal(err error, stage string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error [%s]: %v\n", stage, err)
		os.Exit(1)
	}
}
