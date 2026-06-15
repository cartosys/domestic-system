package main

import (
	"fmt"
	"math/big"

	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/views/uniswap"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
)

// buildTokenList builds the list of swappable tokens from wallet details and watchlist.
func (m model) buildTokenList() []uniswap.TokenOption {
	type heldToken struct {
		balance  *big.Int
		decimals uint8
	}
	held := make(map[string]heldToken, len(m.details.Tokens))
	for _, token := range m.details.Tokens {
		held[token.Symbol] = heldToken{balance: token.Balance, decimals: token.Decimals}
	}

	tokens := []uniswap.TokenOption{{
		Symbol:   "ETH",
		Balance:  m.details.EthWei,
		Decimals: 18,
		IsETH:    true,
	}}

	for _, wt := range m.tokenWatch {
		opt := uniswap.TokenOption{
			Symbol:   wt.Symbol,
			Decimals: wt.Decimals,
			IsETH:    false,
			Address:  wt.Address,
		}
		if h, ok := held[wt.Symbol]; ok {
			opt.Balance = h.balance
			opt.Decimals = h.decimals
		}
		tokens = append(tokens, opt)
	}
	return tokens
}

// chainID returns the connected chain's ID, or nil if there is no connection
// or the lookup failed at connect time. helpers.UniswapAddressesForChain treats
// nil as "assume mainnet", matching the app's existing default network.
func (m *model) chainID() *big.Int {
	if m.ethClient == nil {
		return nil
	}
	return m.ethClient.DetectedChainID
}

// pairResolution carries routing metadata for a resolved token pair.
type pairResolution struct {
	pairAddr   common.Address // V2 pair contract or V3 pool contract
	tokenIn    common.Address
	isV3       bool
	v3Fee      uint32
	v3TokenOut common.Address // explicit tokenOut needed by QuoterV2
}

// resolvePair returns routing metadata for the given token pair.
// Returns ok=false and logs a warning for unsupported pairs.
func (m *model) resolvePair(from, to uniswap.TokenOption) (pairResolution, bool) {
	addrs := helpers.UniswapAddressesForChain(m.chainID())

	if (from.Symbol == "USDC" && to.Symbol == "ETH") || (from.Symbol == "ETH" && to.Symbol == "USDC") {
		tokenIn := addrs.WETH
		if from.Symbol == "USDC" {
			tokenIn = addrs.USDC
		}
		return pairResolution{pairAddr: addrs.USDCWETHPair, tokenIn: tokenIn}, true
	}

	if (from.Symbol == "SPCXon" && to.Symbol == "USDC") || (from.Symbol == "USDC" && to.Symbol == "SPCXon") {
		tokenIn := addrs.SPCXon
		tokenOut := addrs.USDC
		if from.Symbol == "USDC" {
			tokenIn, tokenOut = addrs.USDC, addrs.SPCXon
		}
		if addrs.SPCXonUSDCPoolV3 != (common.Address{}) {
			return pairResolution{
				pairAddr: addrs.SPCXonUSDCPoolV3, tokenIn: tokenIn,
				isV3: true, v3Fee: 10000, v3TokenOut: tokenOut,
			}, true
		}
		return pairResolution{pairAddr: addrs.SPCXonUSDCPair, tokenIn: tokenIn}, true
	}

	if (from.Symbol == "SPCXon" && to.Symbol == "USDT") || (from.Symbol == "USDT" && to.Symbol == "SPCXon") {
		tokenIn := addrs.SPCXon
		tokenOut := addrs.USDT
		if from.Symbol == "USDT" {
			tokenIn, tokenOut = addrs.USDT, addrs.SPCXon
		}
		if addrs.SPCXonUSDTPoolV3 != (common.Address{}) {
			return pairResolution{
				pairAddr: addrs.SPCXonUSDTPoolV3, tokenIn: tokenIn,
				isV3: true, v3Fee: 3000, v3TokenOut: tokenOut,
			}, true
		}
	}

	m.logWarn(fmt.Sprintf("Swap pair %s/%s not supported yet", from.Symbol, to.Symbol))
	return pairResolution{}, false
}

// resolveSwapTokens returns the from/to TokenOptions and whether the pair is valid.
func (m *model) resolveSwapTokens() (from, to uniswap.TokenOption, ok bool) {
	tokens := m.buildTokenList()
	if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
		return
	}
	if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
		return
	}
	from = tokens[m.uniswapFromTokenIdx]
	to = tokens[m.uniswapToTokenIdx]
	ok = from.Symbol != to.Symbol
	return
}

// clearQuoteState resets all swap quote fields.
func (m *model) clearQuoteState() {
	m.uniswapQuote = nil
	m.uniswapQuoteError = ""
	m.uniswapPriceImpactWarn = ""
}

// maybeRequestUniswapQuote triggers a forward swap quote fetch (input → output).
func (m *model) maybeRequestUniswapQuote() tea.Cmd {
	if m.uniswapFromAmount == "" || m.uniswapFromAmount == "0" {
		m.uniswapToAmount = ""
		m.clearQuoteState()
		m.lastQuoteFromAmount = ""
		return nil
	}

	fromToken, toToken, ok := m.resolveSwapTokens()
	if !ok {
		return nil
	}

	if m.lastQuoteFromAmount == m.uniswapFromAmount &&
		m.lastQuoteFromTokenIdx == m.uniswapFromTokenIdx &&
		m.lastQuoteToTokenIdx == m.uniswapToTokenIdx &&
		m.uniswapQuote != nil && m.uniswapToAmount != "" {
		return nil
	}

	amountFloat := new(big.Float)
	if _, ok := amountFloat.SetString(m.uniswapFromAmount); !ok {
		return nil
	}
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
	amountIn, _ := new(big.Float).Mul(amountFloat, multiplier).Int(nil)
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}

	pr, ok := m.resolvePair(fromToken, toToken)
	if !ok {
		return nil
	}

	m.lastQuoteFromAmount = m.uniswapFromAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx
	m.uniswapToAmount = ""
	m.clearQuoteState()
	m.uniswapEstimating = true

	if pr.isV3 {
		m.uniswapLastFee = pr.v3Fee
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		return fetchV3SwapQuote(m.ethClient, addrs.QuoterV2, pr.pairAddr, pr.tokenIn, pr.v3TokenOut, pr.v3Fee, amountIn)
	}
	m.uniswapLastFee = 0
	return fetchUniswapQuote(m.ethClient, pr.pairAddr, pr.tokenIn, amountIn)
}

// maybeRequestReverseUniswapQuote triggers a reverse swap quote fetch (output → required input).
func (m *model) maybeRequestReverseUniswapQuote() tea.Cmd {
	if m.uniswapToAmount == "" || m.uniswapToAmount == "0" {
		m.uniswapFromAmount = ""
		m.clearQuoteState()
		return nil
	}

	fromToken, toToken, ok := m.resolveSwapTokens()
	if !ok {
		return nil
	}

	if m.lastQuoteToAmount == m.uniswapToAmount &&
		m.lastQuoteFromTokenIdx == m.uniswapFromTokenIdx &&
		m.lastQuoteToTokenIdx == m.uniswapToTokenIdx &&
		m.uniswapQuote != nil && m.uniswapFromAmount != "" {
		return nil
	}

	amountOutFloat := new(big.Float)
	if _, ok := amountOutFloat.SetString(m.uniswapToAmount); !ok {
		return nil
	}
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))
	amountOut, _ := new(big.Float).Mul(amountOutFloat, multiplier).Int(nil)
	if amountOut == nil || amountOut.Sign() <= 0 {
		return nil
	}

	pr, ok := m.resolvePair(fromToken, toToken)
	if !ok {
		return nil
	}

	m.lastQuoteToAmount = m.uniswapToAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx
	m.uniswapFromAmount = ""
	m.clearQuoteState()
	m.uniswapEstimating = true
	m.logInfo(fmt.Sprintf("Calculating required input for %s %s", m.uniswapToAmount, toToken.Symbol))

	if pr.isV3 {
		m.uniswapLastFee = pr.v3Fee
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		return fetchV3ReverseSwapQuote(m.ethClient, addrs.QuoterV2, pr.pairAddr, pr.tokenIn, pr.v3TokenOut, pr.v3Fee, amountOut)
	}
	m.uniswapLastFee = 0
	return fetchReverseUniswapQuote(m.ethClient, pr.pairAddr, pr.tokenIn, amountOut, fromToken.Decimals)
}

// fetchUniswapQuote fetches a forward swap quote from Uniswap V2.
func fetchUniswapQuote(client *rpc.Client, pairAddr, tokenInAddr common.Address, amountIn *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}
		quote, err := helpers.GetSwapQuote(client.Client, pairAddr, tokenInAddr, amountIn)
		return uniswapQuoteMsg{quote, err}
	}
}

// fetchReverseUniswapQuote calculates the required input amount for a desired output amount.
func fetchReverseUniswapQuote(client *rpc.Client, pairAddr, tokenInAddr common.Address, amountOut *big.Int, _ uint8) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}
		quote, err := helpers.GetReverseSwapQuote(client.Client, pairAddr, tokenInAddr, amountOut)
		return uniswapQuoteMsg{quote, err}
	}
}

// fetchV3SwapQuote fetches an exact-input quote from the Uniswap V3 QuoterV2.
func fetchV3SwapQuote(client *rpc.Client, quoterV2, poolAddr, tokenIn, tokenOut common.Address, fee uint32, amountIn *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}
		quote, err := helpers.GetV3SwapQuote(client.Client, quoterV2, poolAddr, tokenIn, tokenOut, fee, amountIn)
		return uniswapQuoteMsg{quote, err}
	}
}

// fetchV3ReverseSwapQuote fetches an exact-output quote from the Uniswap V3 QuoterV2.
func fetchV3ReverseSwapQuote(client *rpc.Client, quoterV2, poolAddr, tokenIn, tokenOut common.Address, fee uint32, amountOut *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}
		quote, err := helpers.GetV3ReverseSwapQuote(client.Client, quoterV2, poolAddr, tokenIn, tokenOut, fee, amountOut)
		return uniswapQuoteMsg{quote, err}
	}
}
