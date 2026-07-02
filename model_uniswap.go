package main

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

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
	pairAddr   common.Address // V2 pair contract or V3 pool contract; zero for V4
	tokenIn    common.Address
	version    helpers.PoolVersion
	v3Fee      uint32
	v3TokenOut common.Address // explicit tokenOut needed by QuoterV2
	v4Key      helpers.V4PoolKey
	v4PoolID   common.Hash
}

// pairCacheEntry caches the result of an on-chain factory lookup for a token
// pair (direction-agnostic — tokenIn/v3TokenOut are filled in fresh by
// resolvePairCached for whichever direction is currently selected).
type pairCacheEntry struct {
	resolution pairResolution
	ok         bool
}

// tokenAddrForLookup returns the ERC-20 address to use when querying
// factories/pools for opt — native ETH has no contract, so WETH stands in,
// matching how the router/quoter calls already treat ETH elsewhere.
func tokenAddrForLookup(opt uniswap.TokenOption, weth common.Address) common.Address {
	if opt.IsETH {
		return weth
	}
	return opt.Address
}

// pairCacheKey normalizes two token addresses into an order-independent key,
// since a pool is the same regardless of swap direction.
func pairCacheKey(a, b common.Address) string {
	ah, bh := strings.ToLower(a.Hex()), strings.ToLower(b.Hex())
	if ah > bh {
		ah, bh = bh, ah
	}
	return ah + "_" + bh
}

// resolvePairCached returns a cached pair resolution without any on-chain
// I/O. found=false means this pair has never been looked up this session
// (the caller should dispatch resolvePairOnChain). found=true with ok=false
// means it was already looked up and definitively has no tradable pool.
func (m *model) resolvePairCached(from, to uniswap.TokenOption) (res pairResolution, ok bool, found bool) {
	addrs := helpers.UniswapAddressesForChain(m.chainID())
	tokenInAddr := tokenAddrForLookup(from, addrs.WETH)
	tokenOutAddr := tokenAddrForLookup(to, addrs.WETH)

	entry, found := m.pairCache[pairCacheKey(tokenInAddr, tokenOutAddr)]
	if !found {
		return pairResolution{}, false, false
	}
	if !entry.ok {
		return pairResolution{}, false, true
	}
	res = entry.resolution
	res.tokenIn = tokenInAddr
	if res.version == helpers.PoolVersionV3 {
		res.v3TokenOut = tokenOutAddr
	}
	return res, true, true
}

// resolvePairOnChain dispatches an on-chain Uniswap V2/V3 factory lookup for
// tokenA/tokenB. fromIdx/toIdx capture the dropdown selection active when the
// lookup was dispatched, so handlePairLookupResult can detect a stale result
// if the user changes the selection before the lookup returns.
func resolvePairOnChain(client *rpc.Client, addrs helpers.UniswapNetworkAddresses, tokenA, tokenB common.Address, fromIdx, toIdx int, reverse bool) tea.Cmd {
	return func() tea.Msg {
		key := pairCacheKey(tokenA, tokenB)
		if client == nil || client.Client == nil {
			return pairLookupResultMsg{cacheKey: key, fromIdx: fromIdx, toIdx: toIdx, reverse: reverse, ok: false}
		}
		// 20s (not the 10s V2/V3 factory calls use) to give the V4 tier's
		// bounded recent-block log scan (resolveV4Pool) the same budget
		// FetchPoolKey already uses for an equivalent scan; this only
		// affects lookups that fall through past V2/V3.
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		pool, err := helpers.ResolvePairOnChain(ctx, client.Client, addrs, tokenA, tokenB)
		if err != nil {
			return pairLookupResultMsg{cacheKey: key, fromIdx: fromIdx, toIdx: toIdx, reverse: reverse, ok: false}
		}
		return pairLookupResultMsg{
			cacheKey: key, fromIdx: fromIdx, toIdx: toIdx, reverse: reverse, ok: true,
			resolution: pairResolution{
				pairAddr: pool.PairAddr, version: pool.Version, v3Fee: pool.V3Fee,
				v4Key: pool.V4Key, v4PoolID: pool.V4PoolID,
			},
		}
	}
}

// handlePairLookupResult applies the result of an on-chain factory lookup
// dispatched by resolvePairOnChain: caches it, then resumes whichever quote
// direction triggered the lookup. If the from/to selection has changed since
// the lookup was dispatched, the result is still cached (not wasted) but no
// quote is resumed for it.
func (m *model) handlePairLookupResult(msg pairLookupResultMsg) (tea.Model, tea.Cmd) {
	m.uniswapResolvingPair = false
	m.pairCache[msg.cacheKey] = pairCacheEntry{resolution: msg.resolution, ok: msg.ok}

	if msg.fromIdx != m.uniswapFromTokenIdx || msg.toIdx != m.uniswapToTokenIdx {
		return m, nil
	}
	if !msg.ok {
		if fromToken, toToken, ok := m.resolveSwapTokens(); ok {
			m.uniswapQuoteError = fmt.Sprintf("No Uniswap V2/V3/V4 pool found for %s/%s", fromToken.Symbol, toToken.Symbol)
			m.logWarn(m.uniswapQuoteError)
		}
		return m, nil
	}
	if msg.reverse {
		return m, m.maybeRequestReverseUniswapQuote()
	}
	return m, m.maybeRequestUniswapQuote()
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
	m.uniswapHookWarn = ""
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

	pr, ok, found := m.resolvePairCached(fromToken, toToken)
	if !found {
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		tokenA := tokenAddrForLookup(fromToken, addrs.WETH)
		tokenB := tokenAddrForLookup(toToken, addrs.WETH)
		m.uniswapResolvingPair = true
		m.clearQuoteState()
		return resolvePairOnChain(m.ethClient, addrs, tokenA, tokenB, m.uniswapFromTokenIdx, m.uniswapToTokenIdx, false)
	}
	if !ok {
		m.uniswapQuoteError = fmt.Sprintf("No Uniswap V2/V3/V4 pool found for %s/%s", fromToken.Symbol, toToken.Symbol)
		return nil
	}

	m.lastQuoteFromAmount = m.uniswapFromAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx
	m.uniswapToAmount = ""
	m.clearQuoteState()
	m.uniswapEstimating = true

	m.uniswapLastVersion = pr.version
	switch pr.version {
	case helpers.PoolVersionV3:
		m.uniswapLastFee = pr.v3Fee
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		return fetchV3SwapQuote(m.ethClient, addrs.QuoterV2, pr.pairAddr, pr.tokenIn, pr.v3TokenOut, pr.v3Fee, amountIn)
	case helpers.PoolVersionV4:
		m.uniswapLastV4Key = pr.v4Key
		m.uniswapLastV4PoolID = pr.v4PoolID
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		return fetchV4SwapQuote(m.ethClient, addrs, pr.v4Key, pr.v4PoolID, pr.tokenIn, amountIn)
	default:
		m.uniswapLastFee = 0
		return fetchUniswapQuote(m.ethClient, pr.pairAddr, pr.tokenIn, amountIn)
	}
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

	pr, ok, found := m.resolvePairCached(fromToken, toToken)
	if !found {
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		tokenA := tokenAddrForLookup(fromToken, addrs.WETH)
		tokenB := tokenAddrForLookup(toToken, addrs.WETH)
		m.uniswapResolvingPair = true
		m.clearQuoteState()
		return resolvePairOnChain(m.ethClient, addrs, tokenA, tokenB, m.uniswapFromTokenIdx, m.uniswapToTokenIdx, true)
	}
	if !ok {
		m.uniswapQuoteError = fmt.Sprintf("No Uniswap V2/V3/V4 pool found for %s/%s", fromToken.Symbol, toToken.Symbol)
		return nil
	}

	m.lastQuoteToAmount = m.uniswapToAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx
	m.uniswapFromAmount = ""
	m.clearQuoteState()
	m.uniswapEstimating = true
	m.logInfo(fmt.Sprintf("Calculating required input for %s %s", m.uniswapToAmount, toToken.Symbol))

	m.uniswapLastVersion = pr.version
	switch pr.version {
	case helpers.PoolVersionV3:
		m.uniswapLastFee = pr.v3Fee
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		return fetchV3ReverseSwapQuote(m.ethClient, addrs.QuoterV2, pr.pairAddr, pr.tokenIn, pr.v3TokenOut, pr.v3Fee, amountOut)
	case helpers.PoolVersionV4:
		m.uniswapLastV4Key = pr.v4Key
		m.uniswapLastV4PoolID = pr.v4PoolID
		addrs := helpers.UniswapAddressesForChain(m.chainID())
		return fetchV4ReverseSwapQuote(m.ethClient, addrs, pr.v4Key, pr.v4PoolID, pr.tokenIn, amountOut)
	default:
		m.uniswapLastFee = 0
		return fetchReverseUniswapQuote(m.ethClient, pr.pairAddr, pr.tokenIn, amountOut, fromToken.Decimals)
	}
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

// fetchV4SwapQuote fetches an exact-input quote from the Uniswap V4Quoter.
func fetchV4SwapQuote(client *rpc.Client, addrs helpers.UniswapNetworkAddresses, key helpers.V4PoolKey, poolID common.Hash, tokenIn common.Address, amountIn *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}
		quote, err := helpers.GetV4SwapQuote(client.Client, addrs, key, poolID, tokenIn, amountIn)
		return uniswapQuoteMsg{quote, err}
	}
}

// fetchV4ReverseSwapQuote fetches an exact-output quote from the Uniswap V4Quoter.
func fetchV4ReverseSwapQuote(client *rpc.Client, addrs helpers.UniswapNetworkAddresses, key helpers.V4PoolKey, poolID common.Hash, tokenIn common.Address, amountOut *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}
		quote, err := helpers.GetV4ReverseSwapQuote(client.Client, addrs, key, poolID, tokenIn, amountOut)
		return uniswapQuoteMsg{quote, err}
	}
}
