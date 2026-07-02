package main

import (
	"context"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"time"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/views/uniswap"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// -------------------- RPC --------------------

func connectRPC(url string) tea.Cmd {
	return func() tea.Msg {
		result := rpc.Connect(url)
		return rpcConnectedMsg{client: result.Client, err: result.Error}
	}
}

func initLogViewport() tea.Cmd {
	return func() tea.Msg { return logInitMsg{} }
}

// fetchTokenMetadata looks up an ERC-20 contract's symbol/name/decimals/
// totalSupply for the Watched Tokens add/edit form.
func fetchTokenMetadata(client *rpc.Client, addr common.Address) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return tokenMetadataMsg{address: addr, err: fmt.Errorf("no RPC client")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		symbol, name, decimals, totalSupply, err := rpc.FetchERC20Metadata(ctx, client.Client, addr)
		return tokenMetadataMsg{address: addr, symbol: symbol, name: name, decimals: decimals, totalSupply: totalSupply, err: err}
	}
}

// -------------------- TRANSACTION PACKAGING --------------------

// packageTransaction packages an ETH transfer as an EIP-4527 QR payload.
func packageTransaction(fromAddr, toAddr string, ethAmount string, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		amountFloat := new(big.Float)
		amountFloat.SetString(ethAmount)
		amountWei, _ := new(big.Float).Mul(amountFloat, big.NewFloat(1e18)).Int(nil)

		urStr, txJSON, err := rpc.PackUnsignedTxEIP4527(
			common.HexToAddress(fromAddr),
			common.HexToAddress(toAddr),
			amountWei, 0, nil, rpcURL,
		)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		summary := fmt.Sprintf("ETH Transfer: %s ETH → %s", ethAmount, toAddr)
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527"}
	}
}

// packageSwapTransaction packages a Uniswap V2 swap as an EIP-4527 QR payload.
// chainID picks the network-appropriate router/WETH addresses (mainnet vs Sepolia).
// When the ERC-20 allowance is insufficient an approve tx is packaged at nonce N
// and the swap tx at nonce N+1 so both can be pre-signed in sequence.
func packageSwapTransaction(client *rpc.Client, fromAddr string, fromToken, toToken uniswap.TokenOption, amountIn string, amountOutMin *big.Int, rpcURL string, chainID *big.Int) tea.Cmd {
	return func() tea.Msg {
		addrs := helpers.UniswapAddressesForChain(chainID)
		routerAddress := addrs.Router
		wethAddress := addrs.WETH
		fromAddress := common.HexToAddress(fromAddr)

		amountFloat := new(big.Float)
		amountFloat.SetString(amountIn)
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		amountInBig, _ := new(big.Float).Mul(amountFloat, multiplier).Int(nil)

		outDecimals := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil)
		minOutHuman := new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(outDecimals)).Text('f', 6)

		calldata := buildSwapCalldata(fromToken, toToken, fromAddress, amountInBig, amountOutMin, wethAddress, int64(time.Now().Unix()+1200))
		txValue := big.NewInt(0)
		if fromToken.IsETH {
			txValue = amountInBig
		}

		// Check allowance for ERC-20 input tokens. Treat any RPC error as
		// "allowance unknown" and include an approve step to be safe.
		needsApprove := false
		if !fromToken.IsETH && client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			allowance, err := rpc.ERC20Allowance(ctx, client, fromToken.Address, fromAddress, routerAddress)
			cancel()
			if err != nil || allowance.Cmp(amountInBig) < 0 {
				needsApprove = true
			}
		}

		p, err := rpc.FetchTxParams(rpcURL, fromAddress)
		if err != nil {
			return packageTransactionMsg{err: err}
		}

		swapNonce := p.Nonce
		var approveQRData, approveJSON string
		if needsApprove {
			approveCalldata := buildApproveCalldata(routerAddress, amountInBig)
			approveQRData, approveJSON, err = rpc.BuildUnsignedTxEIP4527(fromAddress, fromToken.Address, big.NewInt(0), 60000, approveCalldata, p.Nonce, p.Tip, p.MaxFee, p.ChainID)
			if err != nil {
				return packageTransactionMsg{err: err}
			}
			swapNonce = p.Nonce + 1
		}

		urStr, txJSON, err := rpc.BuildUnsignedTxEIP4527(fromAddress, routerAddress, txValue, 200000, calldata, swapNonce, p.Tip, p.MaxFee, p.ChainID)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		summary := fmt.Sprintf("Uniswap V2 Swap: %s %s → %s (min %s)\nRouter: %s",
			amountIn, fromToken.Symbol, toToken.Symbol, minOutHuman, routerAddress.Hex())
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527", approveQRData: approveQRData, approveJSON: approveJSON}
	}
}

// packageTerraClaimTx packages a Terra Nullius claim as an EIP-4527 QR payload.
func packageTerraClaimTx(fromAddr, message, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		calldata := helpers.BuildTerraClaimCalldata(message)
		urStr, txJSON, err := rpc.PackUnsignedTxEIP4527(
			common.HexToAddress(fromAddr),
			common.HexToAddress(helpers.TerraContractAddress),
			big.NewInt(0), 0, calldata, rpcURL,
		)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		summary := fmt.Sprintf("Terra Nullius claim: \"%s\"\nContract: %s", message, helpers.TerraContractAddress)
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527"}
	}
}

// -------------------- SIGNED TX BROADCAST --------------------

// broadcastSignedTx relays a pasted, pre-signed raw transaction to the
// connected RPC endpoint via eth_sendRawTransaction.
func broadcastSignedTx(client *rpc.Client, rawHex string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return signedTxBroadcastMsg{err: fmt.Errorf("no RPC client")}
		}
		hash, err := rpc.SendRawTransaction(client, rawHex)
		return signedTxBroadcastMsg{txHash: hash, err: err}
	}
}

// pollTxOnChain checks whether a broadcast transaction has been mined yet.
func pollTxOnChain(client *rpc.Client, txHash string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return signedTxPollResultMsg{err: fmt.Errorf("no RPC client")}
		}
		info, found, err := rpc.GetTransactionOnChain(client, common.HexToHash(txHash))
		return signedTxPollResultMsg{info: info, found: found, err: err}
	}
}

// pasteTxCountdownTick fires once a second to drive the on-chain poll countdown.
func pasteTxCountdownTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(time.Time) tea.Msg {
		return signedTxCountdownTickMsg{}
	})
}

// abiEncodeUint256 ABI-encodes a *big.Int as a 32-byte uint256.
func abiEncodeUint256(v *big.Int) []byte {
	b := make([]byte, 32)
	copy(b[32-len(v.Bytes()):], v.Bytes())
	return b
}

// abiEncodeAddress ABI-encodes a common.Address as a 32-byte padded value.
func abiEncodeAddress(addr common.Address) []byte {
	b := make([]byte, 32)
	copy(b[12:], addr[:])
	return b
}

// buildApproveCalldata ABI-encodes approve(spender, amount) for an ERC-20 token.
// Selector 0x095ea7b3: approve(address,uint256)
func buildApproveCalldata(spender common.Address, amount *big.Int) []byte {
	var d []byte
	d = append(d, 0x09, 0x5e, 0xa7, 0xb3)
	d = append(d, abiEncodeAddress(spender)...)
	d = append(d, abiEncodeUint256(amount)...)
	return d
}

// buildSwapCalldata builds ABI-encoded calldata for the appropriate Uniswap V2 swap function.
func buildSwapCalldata(fromToken, toToken uniswap.TokenOption, to common.Address, amountIn, amountOutMin *big.Int, weth common.Address, deadline int64) []byte {
	dl := big.NewInt(deadline)
	if fromToken.IsETH {
		// swapExactETHForTokens — selector 0x7ff36ab5
		var d []byte
		d = append(d, 0x7f, 0xf3, 0x6a, 0xb5)
		d = append(d, abiEncodeUint256(amountOutMin)...)
		d = append(d, abiEncodeUint256(big.NewInt(128))...)
		d = append(d, abiEncodeAddress(to)...)
		d = append(d, abiEncodeUint256(dl)...)
		d = append(d, abiEncodeUint256(big.NewInt(2))...)
		d = append(d, abiEncodeAddress(weth)...)
		d = append(d, abiEncodeAddress(toToken.Address)...)
		return d
	}
	if toToken.IsETH {
		// swapExactTokensForETH — selector 0x18cbafe5
		var d []byte
		d = append(d, 0x18, 0xcb, 0xaf, 0xe5)
		d = append(d, abiEncodeUint256(amountIn)...)
		d = append(d, abiEncodeUint256(amountOutMin)...)
		d = append(d, abiEncodeUint256(big.NewInt(160))...)
		d = append(d, abiEncodeAddress(to)...)
		d = append(d, abiEncodeUint256(dl)...)
		d = append(d, abiEncodeUint256(big.NewInt(2))...)
		d = append(d, abiEncodeAddress(fromToken.Address)...)
		d = append(d, abiEncodeAddress(weth)...)
		return d
	}
	// swapExactTokensForTokens — selector 0x38ed1739
	var d []byte
	d = append(d, 0x38, 0xed, 0x17, 0x39)
	d = append(d, abiEncodeUint256(amountIn)...)
	d = append(d, abiEncodeUint256(amountOutMin)...)
	d = append(d, abiEncodeUint256(big.NewInt(160))...)
	d = append(d, abiEncodeAddress(to)...)
	d = append(d, abiEncodeUint256(dl)...)
	d = append(d, abiEncodeUint256(big.NewInt(2))...)
	d = append(d, abiEncodeAddress(fromToken.Address)...)
	d = append(d, abiEncodeAddress(toToken.Address)...)
	return d
}

// packageSwapTransactionV3 packages a Uniswap V3 exactInputSingle swap as an EIP-4527 QR payload.
// fee is the pool fee tier in hundredths of a bip (e.g. 10000 = 1%, 3000 = 0.3%).
// When the ERC-20 allowance is insufficient an approve tx is packaged at nonce N
// and the swap tx at nonce N+1 so both can be pre-signed in sequence.
func packageSwapTransactionV3(client *rpc.Client, fromAddr string, fromToken, toToken uniswap.TokenOption, fee uint32, amountIn string, amountOutMin *big.Int, rpcURL string, chainID *big.Int) tea.Cmd {
	return func() tea.Msg {
		addrs := helpers.UniswapAddressesForChain(chainID)
		routerAddress := addrs.SwapRouterV3
		fromAddress := common.HexToAddress(fromAddr)

		amountFloat := new(big.Float)
		amountFloat.SetString(amountIn)
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		amountInBig, _ := new(big.Float).Mul(amountFloat, multiplier).Int(nil)

		outDecimals := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil)
		minOutHuman := new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(outDecimals)).Text('f', 6)

		calldata := buildV3SwapCalldata(fromToken, toToken, fromAddress, amountInBig, amountOutMin, addrs.WETH, fee)
		txValue := big.NewInt(0)
		if fromToken.IsETH {
			txValue = amountInBig
		}

		// Check allowance for ERC-20 input tokens. Treat any RPC error as
		// "allowance unknown" and include an approve step to be safe.
		needsApprove := false
		if !fromToken.IsETH && client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			allowance, err := rpc.ERC20Allowance(ctx, client, fromToken.Address, fromAddress, routerAddress)
			cancel()
			if err != nil || allowance.Cmp(amountInBig) < 0 {
				needsApprove = true
			}
		}

		p, err := rpc.FetchTxParams(rpcURL, fromAddress)
		if err != nil {
			return packageTransactionMsg{err: err}
		}

		swapNonce := p.Nonce
		var approveQRData, approveJSON string
		if needsApprove {
			approveCalldata := buildApproveCalldata(routerAddress, amountInBig)
			approveQRData, approveJSON, err = rpc.BuildUnsignedTxEIP4527(fromAddress, fromToken.Address, big.NewInt(0), 60000, approveCalldata, p.Nonce, p.Tip, p.MaxFee, p.ChainID)
			if err != nil {
				return packageTransactionMsg{err: err}
			}
			swapNonce = p.Nonce + 1
		}

		urStr, txJSON, err := rpc.BuildUnsignedTxEIP4527(fromAddress, routerAddress, txValue, 200000, calldata, swapNonce, p.Tip, p.MaxFee, p.ChainID)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		feeLabel := fmt.Sprintf("%.2f%%", float64(fee)/10000.0)
		summary := fmt.Sprintf("Uniswap V3 Swap (%s): %s %s → %s (min %s)\nRouter: %s",
			feeLabel, amountIn, fromToken.Symbol, toToken.Symbol, minOutHuman, routerAddress.Hex())
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527", approveQRData: approveQRData, approveJSON: approveJSON}
	}
}

// buildV3SwapCalldata ABI-encodes exactInputSingle for SwapRouter02.
// Selector 0x04e45aaf: exactInputSingle((address,address,uint24,address,uint256,uint256,uint160))
// SwapRouter02 omits the deadline field present in SwapRouter v1.
func buildV3SwapCalldata(fromToken, toToken uniswap.TokenOption, to common.Address, amountIn, amountOutMin *big.Int, weth common.Address, fee uint32) []byte {
	tokenIn := fromToken.Address
	tokenOut := toToken.Address
	if fromToken.IsETH {
		tokenIn = weth
	}
	if toToken.IsETH {
		tokenOut = weth
	}

	var d []byte
	d = append(d, 0x04, 0xe4, 0x5a, 0xaf) // exactInputSingle selector
	d = append(d, abiEncodeAddress(tokenIn)...)
	d = append(d, abiEncodeAddress(tokenOut)...)
	d = append(d, abiEncodeUint256(new(big.Int).SetUint64(uint64(fee)))...)
	d = append(d, abiEncodeAddress(to)...)
	d = append(d, abiEncodeUint256(amountIn)...)
	d = append(d, abiEncodeUint256(amountOutMin)...)
	d = append(d, abiEncodeUint256(big.NewInt(0))...) // sqrtPriceLimitX96 = 0 (no limit)
	return d
}

// -------------------- UNISWAP V4 SWAP PACKAGING --------------------
//
// [VERIFY] Everything in this section encodes calldata against Uniswap's
// Universal Router V4_SWAP path. The action-ID bytes (0x06/0x0c/0x0f) and the
// V4_SWAP command byte (0x10) are cross-checked against Uniswap's official
// GitHub source (Commands.sol, Actions.sol) as of this writing. The
// ExactInputSingleParams struct shape (including a minHopPriceX36 field) is
// taken from the same source's current main branch — since Solidity ABI
// encoding is offset-sensitive, if the specific deployed bytecode at
// helpers.UniswapNetworkAddresses.UniversalRouter predates that field, this
// encoding will not match what the contract expects and the resulting
// unsigned tx will fail on execution (it is never auto-broadcast — the user
// reviews/signs it — so this is a functional-correctness risk, not a
// fund-loss one, but it must be confirmed against the deployed contract's
// verified source on Etherscan before being trusted as correct).

const v4SwapEncodingABI = `[
  {
    "inputs": [{
      "name": "params", "type": "tuple",
      "components": [
        {"name": "poolKey", "type": "tuple", "components": [
          {"name": "currency0", "type": "address"},
          {"name": "currency1", "type": "address"},
          {"name": "fee", "type": "uint24"},
          {"name": "tickSpacing", "type": "int24"},
          {"name": "hooks", "type": "address"}
        ]},
        {"name": "zeroForOne", "type": "bool"},
        {"name": "amountIn", "type": "uint128"},
        {"name": "amountOutMinimum", "type": "uint128"},
        {"name": "minHopPriceX36", "type": "uint256"},
        {"name": "hookData", "type": "bytes"}
      ]
    }],
    "name": "encodeExactInputSingle",
    "outputs": [], "stateMutability": "pure", "type": "function"
  },
  {
    "inputs": [
      {"name": "currency", "type": "address"},
      {"name": "amount", "type": "uint256"}
    ],
    "name": "encodeCurrencyAmount",
    "outputs": [], "stateMutability": "pure", "type": "function"
  },
  {
    "inputs": [
      {"name": "actions", "type": "bytes"},
      {"name": "params", "type": "bytes[]"}
    ],
    "name": "encodeActionsParams",
    "outputs": [], "stateMutability": "pure", "type": "function"
  },
  {
    "inputs": [
      {"name": "commands", "type": "bytes"},
      {"name": "inputs", "type": "bytes[]"},
      {"name": "deadline", "type": "uint256"}
    ],
    "name": "execute",
    "outputs": [], "stateMutability": "nonpayable", "type": "function"
  }
]`

// v4ActionSwapExactInSingle, v4ActionSettleAll, v4ActionTakeAll are Actions.sol
// action IDs (Uniswap/v4-periphery). v4CommandV4Swap is Commands.sol's V4_SWAP
// command ID (Uniswap/universal-router).
const (
	v4ActionSwapExactInSingle = 0x06
	v4ActionSettleAll         = 0x0c
	v4ActionTakeAll           = 0x0f
	v4CommandV4Swap           = 0x10
)

// v4CurrencyForToken returns the V4 Currency (an address, zero for native ETH)
// for a swap token, matching the zero-address-means-native convention already
// used throughout this codebase's V4 code (helpers/uniswap_v4_listener.go).
func v4CurrencyForToken(t uniswap.TokenOption) common.Address {
	if t.IsETH {
		return common.Address{}
	}
	return t.Address
}

// packStripSelector packs args against method in parsedABI and strips the
// leading 4-byte method selector, yielding the raw abi.encode(...) bytes a
// Solidity contract's abi.encode(struct) / abi.encode(a, b) would produce —
// there is no method selector for those (only real external function calls
// have one), so v4SwapEncodingABI's helper "functions" exist purely to reuse
// go-ethereum's struct/dynamic-array ABI packing instead of hand-deriving
// offsets, matching the same approach already used for V4Quoter's calldata
// (helpers/uniswap_v4_quote.go) where a nested dynamic bytes field made
// manual packing error-prone.
func packStripSelector(parsedABI *abi.ABI, method string, args ...interface{}) ([]byte, error) {
	packed, err := parsedABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	return packed[4:], nil
}

// buildV4SwapCalldata ABI-encodes a Universal Router execute() call carrying
// a single V4_SWAP command: SWAP_EXACT_IN_SINGLE, then SETTLE_ALL (pay the
// input) and TAKE_ALL (receive the output) — the standard V4 single-hop
// exact-input swap pattern.
func buildV4SwapCalldata(fromToken, toToken uniswap.TokenOption, key helpers.V4PoolKey, amountIn, amountOutMin *big.Int, deadline int64) ([]byte, error) {
	parsedABI, err := abi.JSON(strings.NewReader(v4SwapEncodingABI))
	if err != nil {
		return nil, fmt.Errorf("parse V4 swap encoding ABI: %w", err)
	}

	tokenIn := v4CurrencyForToken(fromToken)
	tokenOut := v4CurrencyForToken(toToken)
	zeroForOne := tokenIn == key.Currency0

	swapParams, err := packStripSelector(&parsedABI, "encodeExactInputSingle", struct {
		PoolKey struct {
			Currency0   common.Address
			Currency1   common.Address
			Fee         *big.Int
			TickSpacing *big.Int
			Hooks       common.Address
		}
		ZeroForOne       bool
		AmountIn         *big.Int
		AmountOutMinimum *big.Int
		MinHopPriceX36   *big.Int
		HookData         []byte
	}{
		PoolKey: struct {
			Currency0   common.Address
			Currency1   common.Address
			Fee         *big.Int
			TickSpacing *big.Int
			Hooks       common.Address
		}{
			Currency0:   key.Currency0,
			Currency1:   key.Currency1,
			Fee:         new(big.Int).SetUint64(uint64(key.Fee)),
			TickSpacing: big.NewInt(int64(key.TickSpacing)),
			Hooks:       key.Hooks,
		},
		ZeroForOne:       zeroForOne,
		AmountIn:         amountIn,
		AmountOutMinimum: amountOutMin,
		MinHopPriceX36:   big.NewInt(0), // no additional per-hop price floor beyond amountOutMinimum
		HookData:         []byte{},
	})
	if err != nil {
		return nil, fmt.Errorf("encode ExactInputSingleParams: %w", err)
	}

	settleParams, err := packStripSelector(&parsedABI, "encodeCurrencyAmount", tokenIn, amountIn)
	if err != nil {
		return nil, fmt.Errorf("encode SETTLE_ALL params: %w", err)
	}
	takeParams, err := packStripSelector(&parsedABI, "encodeCurrencyAmount", tokenOut, amountOutMin)
	if err != nil {
		return nil, fmt.Errorf("encode TAKE_ALL params: %w", err)
	}

	actions := []byte{v4ActionSwapExactInSingle, v4ActionSettleAll, v4ActionTakeAll}
	v4SwapInput, err := packStripSelector(&parsedABI, "encodeActionsParams", actions, [][]byte{swapParams, settleParams, takeParams})
	if err != nil {
		return nil, fmt.Errorf("encode actions/params: %w", err)
	}

	commands := []byte{v4CommandV4Swap}
	calldata, err := parsedABI.Pack("execute", commands, [][]byte{v4SwapInput}, big.NewInt(deadline))
	if err != nil {
		return nil, fmt.Errorf("encode execute(): %w", err)
	}
	return calldata, nil
}

// packageSwapTransactionV4 packages a Uniswap V4 single-hop swap as an
// EIP-4527 QR payload via the Universal Router. Unlike V2/V3 (direct
// router approval), V4/Universal Router spends through Permit2, so a
// non-ETH input token needs up to two approval steps ahead of the swap:
// ERC20.approve(Permit2) if Permit2's own allowance from the token is
// insufficient, then Permit2.approve(router, token, amount, expiry) if
// Permit2's recorded allowance for the router is insufficient/expired.
// Both are optional/independent — a wallet that already approved Permit2
// unlimited, and has a live Permit2->router approval, needs neither.
func packageSwapTransactionV4(client *rpc.Client, fromAddr string, fromToken, toToken uniswap.TokenOption, key helpers.V4PoolKey, amountIn string, amountOutMin *big.Int, rpcURL string, chainID *big.Int) tea.Cmd {
	return func() tea.Msg {
		addrs := helpers.UniswapAddressesForChain(chainID)
		routerAddress := addrs.UniversalRouter
		fromAddress := common.HexToAddress(fromAddr)

		amountFloat := new(big.Float)
		amountFloat.SetString(amountIn)
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		amountInBig, _ := new(big.Float).Mul(amountFloat, multiplier).Int(nil)

		outDecimals := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil)
		minOutHuman := new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(outDecimals)).Text('f', 6)

		deadline := int64(time.Now().Unix() + 1200)
		calldata, err := buildV4SwapCalldata(fromToken, toToken, key, amountInBig, amountOutMin, deadline)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		txValue := big.NewInt(0)
		if fromToken.IsETH {
			txValue = amountInBig
		}

		// Two independent, optional approval steps ahead of the swap (see
		// doc comment above). Treat any RPC error as "allowance unknown"
		// and include the step to be safe, matching V2/V3's existing
		// needsApprove behavior.
		needsERC20Approve := false
		needsPermit2Approve := false
		if !fromToken.IsETH && client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			erc20Allowance, err := rpc.ERC20Allowance(ctx, client, fromToken.Address, fromAddress, rpc.Permit2Address)
			cancel()
			if err != nil || erc20Allowance.Cmp(amountInBig) < 0 {
				needsERC20Approve = true
			}

			ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			permit2Amount, permit2Expiration, err := rpc.Permit2Allowance(ctx2, client, fromToken.Address, fromAddress, routerAddress)
			cancel2()
			nowUnix := uint64(time.Now().Unix())
			if err != nil || permit2Amount.Cmp(amountInBig) < 0 || permit2Expiration <= nowUnix {
				needsPermit2Approve = true
			}
		}

		if needsERC20Approve && needsPermit2Approve {
			// This app's QR flow (approveQRData/approveJSON) only carries one
			// pre-swap approval step; V4/Universal Router's Permit2 model can
			// need two independent ones (ERC20→Permit2, then Permit2→router).
			// Silently packaging only one would produce a swap tx that fails
			// on execution with no indication why — fail loud instead and
			// tell the user how to get there in two passes.
			return packageTransactionMsg{err: fmt.Errorf(
				"%s needs two approvals before this V4 swap can run: first ERC20→Permit2, then Permit2→Universal Router. "+
					"This app only packages one approve step per swap attempt — submit this swap once to sign the first approval, "+
					"wait for it to confirm, then reopen the swap to get the second approval + swap",
				fromToken.Symbol)}
		}

		p, err := rpc.FetchTxParams(rpcURL, fromAddress)
		if err != nil {
			return packageTransactionMsg{err: err}
		}

		swapNonce := p.Nonce
		var approveQRData, approveJSON string
		if needsERC20Approve {
			approveCalldata := buildApproveCalldata(rpc.Permit2Address, amountInBig)
			approveQRData, approveJSON, err = rpc.BuildUnsignedTxEIP4527(fromAddress, fromToken.Address, big.NewInt(0), 60000, approveCalldata, p.Nonce, p.Tip, p.MaxFee, p.ChainID)
			if err != nil {
				return packageTransactionMsg{err: err}
			}
			swapNonce = p.Nonce + 1
		} else if needsPermit2Approve {
			permit2ExpiryU48 := uint64(time.Now().Unix() + 60*60*24*30) // 30 days
			permit2Calldata, perr := buildPermit2ApproveCalldata(fromToken.Address, routerAddress, amountInBig, permit2ExpiryU48)
			if perr != nil {
				return packageTransactionMsg{err: perr}
			}
			approveQRData, approveJSON, err = rpc.BuildUnsignedTxEIP4527(fromAddress, rpc.Permit2Address, big.NewInt(0), 80000, permit2Calldata, p.Nonce, p.Tip, p.MaxFee, p.ChainID)
			if err != nil {
				return packageTransactionMsg{err: err}
			}
			swapNonce = p.Nonce + 1
		}

		urStr, txJSON, err := rpc.BuildUnsignedTxEIP4527(fromAddress, routerAddress, txValue, 300000, calldata, swapNonce, p.Tip, p.MaxFee, p.ChainID)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		feeLabel := fmt.Sprintf("%.2f%%", float64(key.Fee)/10000.0)
		hookNote := ""
		if key.Hooks != (common.Address{}) {
			hookNote = fmt.Sprintf("\nHook: %s (pool may enforce KYC/allowlist checks)", key.Hooks.Hex())
		}
		summary := fmt.Sprintf("Uniswap V4 Swap (%s): %s %s → %s (min %s)\nUniversal Router: %s%s",
			feeLabel, amountIn, fromToken.Symbol, toToken.Symbol, minOutHuman, routerAddress.Hex(), hookNote)
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527", approveQRData: approveQRData, approveJSON: approveJSON}
	}
}

// buildPermit2ApproveCalldata ABI-encodes Permit2's
// approve(address token, address spender, uint160 amount, uint48 expiration).
// [VERIFY] against Permit2's verified Etherscan source alongside rpc.Permit2Address.
func buildPermit2ApproveCalldata(token, spender common.Address, amount *big.Int, expiration uint64) ([]byte, error) {
	const permit2ApproveABI = `[{"inputs":[{"name":"token","type":"address"},{"name":"spender","type":"address"},{"name":"amount","type":"uint160"},{"name":"expiration","type":"uint48"}],"name":"approve","outputs":[],"stateMutability":"nonpayable","type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(permit2ApproveABI))
	if err != nil {
		return nil, err
	}
	return parsedABI.Pack("approve", token, spender, amount, expiration)
}

// -------------------- CLIPBOARD --------------------

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		if clipboard.WriteAll(text) == nil {
			return clipboardCopiedMsg{}
		}
		return nil
	}
}

func copyPoolIDToClipboard(poolID string) tea.Cmd {
	return func() tea.Msg {
		if clipboard.WriteAll(poolID) == nil {
			return poolIDCopiedMsg{}
		}
		return nil
	}
}

func copyTxJsonToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		if clipboard.WriteAll(text) == nil {
			return txJsonCopiedMsg{}
		}
		return nil
	}
}

func clearClipboardMsg() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return struct{ clearClipboard bool }{true}
	})
}

// -------------------- ENS --------------------

func lookupENS(client *rpc.Client, address string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ensLookupResultMsg{address: address, err: fmt.Errorf("no RPC client")}
		}
		result := helpers.LookupENS(address, client.URL)
		return ensLookupResultMsg{address: address, ensName: result.Name, err: result.Error, debugInfo: result.DebugInfo}
	}
}

func resolveENS(client *rpc.Client, ensName string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ensForwardResolveMsg{ensName: ensName, err: fmt.Errorf("no RPC client")}
		}
		result := helpers.ResolveENS(ensName, client.URL)
		return ensForwardResolveMsg{ensName: ensName, address: result.Name, err: result.Error, debugInfo: result.DebugInfo}
	}
}

// -------------------- TERRA NULLIUS --------------------

func fetchTerraNumberOfClaims(client *rpc.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return terraNullClaimsCountMsg{nil, fmt.Errorf("no RPC client")}
		}
		count, err := helpers.GetTerraNumberOfClaims(client.Client)
		return terraNullClaimsCountMsg{count, err}
	}
}

func fetchTerraClaim(client *rpc.Client, index *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return terraNullClaimQueryMsg{nil, fmt.Errorf("no RPC client")}
		}
		result, err := helpers.GetTerraClaim(client.Client, index)
		return terraNullClaimQueryMsg{result, err}
	}
}

// -------------------- UNISWAP / POOL --------------------

func fetchLiquidityPositions(rpcURL string, ownerAddr common.Address) tea.Cmd {
	return func() tea.Msg {
		positions, nftCount, diags, err := helpers.GetLiquidityPositions(rpcURL, ownerAddr)
		return liquidityPositionsMsg{positions: positions, nftCount: nftCount, diagnostics: diags, err: err}
	}
}

func fetchPoolInfo(rpcURL, poolIDHex string) tea.Cmd {
	return func() tea.Msg {
		info, err := helpers.FetchPoolInfo(rpcURL, common.HexToHash(poolIDHex))
		return poolInfoResultMsg{poolID: poolIDHex, info: info, err: err}
	}
}

func fetchPoolKey(rpcURL, poolIDHex string) tea.Cmd {
	return func() tea.Msg {
		key, err := helpers.FetchPoolKey(rpcURL, common.HexToHash(poolIDHex))
		return poolKeyResultMsg{poolID: poolIDHex, key: key, err: err}
	}
}

// -------------------- BROWSER --------------------

// openInBrowser opens url in the system default browser.
// Tries xdg-open (Linux), open (macOS), then x-www-browser as fallback.
func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		for _, cmd := range []string{"xdg-open", "open", "x-www-browser"} {
			if path, err := exec.LookPath(cmd); err == nil {
				_ = exec.Command(path, url).Start()
				return nil
			}
		}
		return nil
	}
}

// -------------------- OSC 8 HYPERLINK PARSING --------------------

// logLinkRegion describes the visual column range of a single OSC 8 hyperlink in a log line.
type logLinkRegion struct {
	startCol int
	endCol   int
	url      string
}

// parseOSC8Links returns the visual column range of every OSC 8 hyperlink in line.
// Column positions use ansi.StringWidth so ANSI SGR codes are treated as zero-width.
func parseOSC8Links(line string) []logLinkRegion {
	const osc8Prefix = "\x1b]8;;"
	var regions []logLinkRegion
	visualCol := 0
	remaining := line

	for {
		idx := strings.Index(remaining, osc8Prefix)
		if idx < 0 {
			break
		}
		visualCol += ansi.StringWidth(remaining[:idx])
		after := remaining[idx+len(osc8Prefix):]

		belIdx := strings.IndexByte(after, '\x07')
		if belIdx < 0 {
			break
		}
		url := after[:belIdx]
		afterBEL := after[belIdx+1:]

		if url == "" {
			remaining = afterBEL
			continue
		}

		resetIdx := strings.Index(afterBEL, osc8Prefix)
		if resetIdx < 0 {
			break
		}
		displayWidth := ansi.StringWidth(afterBEL[:resetIdx])
		regions = append(regions, logLinkRegion{startCol: visualCol, endCol: visualCol + displayWidth, url: url})
		visualCol += displayWidth

		afterDisplay := afterBEL[resetIdx+len(osc8Prefix):]
		resetBEL := strings.IndexByte(afterDisplay, '\x07')
		if resetBEL < 0 {
			break
		}
		remaining = afterDisplay[resetBEL+1:]
	}
	return regions
}

// urlAtCol returns the URL of the OSC 8 hyperlink at visual column col, or "".
func urlAtCol(line string, col int) string {
	for _, r := range parseOSC8Links(line) {
		if col >= r.startCol && col < r.endCol {
			return r.url
		}
	}
	return ""
}
