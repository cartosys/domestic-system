package main

import (
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
			amountWei, 21000, nil, rpcURL,
		)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		summary := fmt.Sprintf("ETH Transfer: %s ETH → %s", ethAmount, toAddr)
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527"}
	}
}

// packageSwapTransaction packages a Uniswap V2 swap as an EIP-4527 QR payload.
func packageSwapTransaction(fromAddr string, fromToken, toToken uniswap.TokenOption, amountIn string, amountOutMin *big.Int, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		routerAddress := common.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
		wethAddress := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
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

		urStr, txJSON, err := rpc.PackUnsignedTxEIP4527(fromAddress, routerAddress, txValue, 200000, calldata, rpcURL)
		if err != nil {
			return packageTransactionMsg{err: err}
		}
		summary := fmt.Sprintf("Uniswap V2 Swap: %s %s → %s (min %s)\nRouter: %s",
			amountIn, fromToken.Symbol, toToken.Symbol, minOutHuman, routerAddress.Hex())
		return packageTransactionMsg{txDisplay: summary, txJSON: txJSON, qrData: urStr, format: "EIP-4527"}
	}
}

// packageTerraClaimTx packages a Terra Nullius claim as an EIP-4527 QR payload.
func packageTerraClaimTx(fromAddr, message, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		calldata := helpers.BuildTerraClaimCalldata(message)
		urStr, txJSON, err := rpc.PackUnsignedTxEIP4527(
			common.HexToAddress(fromAddr),
			common.HexToAddress(helpers.TerraContractAddress),
			big.NewInt(0), 100000, calldata, rpcURL,
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
