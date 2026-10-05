package main

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/store"
	"charm-wallet-tui/styles"
	"charm-wallet-tui/views/uniswap"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	for _, wt := range m.tokenWatchForActiveChain() {
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

// tokenWatchForActiveChain returns m.tokenWatch filtered to the connected
// chain, so balance loads, the tx indexer, the Watched Tokens page, and the
// Uniswap token picker only ever see the current network's addresses.
func (m *model) tokenWatchForActiveChain() []rpc.WatchedToken {
	return tokensForChain(m.tokenWatch, m.chainID())
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

// poolListVisible reports whether the Pool List sub-view is on screen.
func (m model) poolListVisible() bool {
	return m.activePage == config.PageUniswap && m.uniswapShowingPoolList && !m.uniswapShowingLiquidity
}

// poolDetailView adapts the shared pool-detail cache entry for poolID to the view payload.
func (m model) poolDetailView(poolID string) uniswap.PoolDetailView {
	st, ok := m.poolDetails[poolID]
	if !ok {
		return uniswap.PoolDetailView{}
	}
	return uniswap.PoolDetailView{Loading: st.loading, Err: st.err, Data: st.data}
}

// poolPageSize is how many pools each lazy-load query fetches and renders.
const poolPageSize = 20

// poolViewKind identifies which lazily-paged pool view a page belongs to.
type poolViewKind uint8

const (
	poolViewV4Events poolViewKind = iota
	poolViewList
)

// poolPager holds the lazily-loaded rows for one pool view. Every request bumps seq;
// only the response carrying the current seq is applied, so results from superseded
// requests (older search keystrokes, overlapping reloads) are dropped.
type poolPager struct {
	rows       []store.PoolRow
	total      int
	hasMore    bool
	loading    bool
	query      string
	seq        int
	cards      map[string]string // collapsed-card render cache by pool ID
	cardsWidth int

	// expanded is the pool whose detail view currently fills the panel (nil = list shown).
	// It is a copy so the detail survives reloads that drop the row from the loaded page.
	expanded    *store.PoolRow
	listYOffset int // list scroll position to restore when the detail is closed
}

func (m *model) poolViewport(view poolViewKind) *viewport.Model {
	if view == poolViewList {
		return &m.poolListViewport
	}
	return &m.v4EventsViewport
}

func (m *model) pager(view poolViewKind) *poolPager {
	if view == poolViewList {
		return &m.poolListPager
	}
	return &m.v4Pager
}

// loadPoolPage requests limit rows at offset for view, superseding any in-flight request.
func (m *model) loadPoolPage(view poolViewKind, offset, limit int) tea.Cmd {
	if m.eventStore == nil {
		return nil
	}
	p := m.pager(view)
	p.seq++
	p.loading = true
	return loadPoolPageCmd(m.eventStore, view, p.query, offset, limit, p.seq)
}

// reloadPoolPager re-queries view from the top, keeping at least as many rows as are
// already loaded so scroll position survives (used when a new pool is indexed).
func (m *model) reloadPoolPager(view poolViewKind) tea.Cmd {
	return m.loadPoolPage(view, 0, max(poolPageSize, len(m.pager(view).rows)))
}

// handlePoolPage applies a page result: offset 0 replaces the rows, later offsets append.
func (m *model) handlePoolPage(msg poolPageMsg) (tea.Model, tea.Cmd) {
	p := m.pager(msg.view)
	if msg.seq != p.seq {
		return m, nil
	}
	p.loading = false
	if msg.err != nil {
		p.hasMore = false
		m.logError(fmt.Sprintf("Pool list: query failed: %s", msg.err.Error()))
		m.refreshPoolView(msg.view)
		return m, nil
	}
	if msg.offset == 0 {
		// Drop cached cards whose row data changed (e.g. new swap counts).
		old := make(map[string]store.PoolRow, len(p.rows))
		for _, r := range p.rows {
			old[r.PoolID] = r
		}
		for _, r := range msg.rows {
			if o, ok := old[r.PoolID]; ok && o != r {
				delete(p.cards, r.PoolID)
			}
		}
		p.rows = msg.rows
	} else {
		p.rows = append(p.rows, msg.rows...)
	}
	p.total = msg.total
	p.hasMore = len(msg.rows) > 0 && len(p.rows) < p.total
	m.refreshPoolView(msg.view)
	return m, nil
}

// maybeLoadMorePools fetches the next page for each visible pool view once its viewport
// is scrolled to within one screen of the end of the loaded cards. Called after every
// Update, so it covers every scroll path (keys, wheel, scrollbar drag) and keeps
// filling a tall viewport until it overflows.
func (m *model) maybeLoadMorePools() tea.Cmd {
	if m.activePage != config.PageUniswap || m.uniswapShowingLiquidity {
		return nil
	}
	view, vp := poolViewV4Events, m.v4EventsViewport
	switch {
	case m.uniswapShowingPoolList:
		view, vp = poolViewList, m.poolListViewport
	case !m.poolEventMonitorActive:
		return nil
	}
	p := m.pager(view)
	if !p.hasMore || p.loading || p.expanded != nil {
		return nil
	}
	if vp.YOffset+vp.Height < vp.TotalLineCount()-vp.Height {
		return nil
	}
	return m.loadPoolPage(view, len(p.rows), poolPageSize)
}

// poolCardCache returns view's collapsed-card cache, cleared if the render width changed.
func (m *model) poolCardCache(view poolViewKind) map[string]string {
	p := m.pager(view)
	if p.cards == nil || p.cardsWidth != m.w {
		p.cards = make(map[string]string)
		p.cardsWidth = m.w
	}
	return p.cards
}

// poolListFooter is the status line appended below the loaded cards.
func poolListFooter(p *poolPager) string {
	switch {
	case len(p.rows) == 0:
		return ""
	case p.loading:
		return "\n" + lipgloss.NewStyle().Foreground(styles.CMuted).Render("  Loading more pools…")
	case !p.hasMore:
		return "\n" + lipgloss.NewStyle().Foreground(styles.CMuted).Render("  — end of list —")
	}
	return ""
}

// refreshPoolView rebuilds one pool view's viewport content and card hit-test spans.
// With a pool expanded, the content is only that pool's detail card, stretched to fill
// the viewport; otherwise it is the list, where collapsed cards come from the render
// cache so only new or changed cards render.
func (m *model) refreshPoolView(view poolViewKind) {
	p := m.pager(view)
	vp := m.poolViewport(view)

	var content string
	var spans []uniswap.CardSpan
	switch {
	case p.expanded != nil:
		detail := m.poolDetailView(p.expanded.PoolID)
		detail.MinHeight = vp.Height
		detail.Width = vp.Width
		content, spans = uniswap.PoolCards(m.w-2, []store.PoolRow{*p.expanded}, "", p.expanded.PoolID, detail, nil)
	case view == poolViewV4Events:
		content, spans = uniswap.V4EventsContent(m.w-2, p.rows, "", uniswap.PoolDetailView{}, m.poolCardCache(view))
		content += poolListFooter(p)
	default:
		emptyMsg := "No indexed pools yet — press p to start the pool event monitor"
		switch {
		case m.eventStore == nil:
			emptyMsg = "Event store unavailable — pools can't be listed"
		case p.loading:
			emptyMsg = "Loading pools…"
		case p.query != "":
			emptyMsg = "No pools match your search"
		}
		content, spans = uniswap.PoolCards(m.w-2, p.rows, emptyMsg, "", uniswap.PoolDetailView{}, m.poolCardCache(view))
		content += poolListFooter(p)
	}

	if view == poolViewV4Events {
		m.v4EventsContent, m.v4EventsSpans = content, spans
	} else {
		m.poolListContent, m.poolListSpans = content, spans
	}
	vp.SetContent(content)
}

// refreshPoolViewports rebuilds both pool views (cheap: collapsed cards are cached).
func (m *model) refreshPoolViewports() {
	m.refreshPoolView(poolViewV4Events)
	m.refreshPoolView(poolViewList)
}

// openPoolDetail makes row's detail view fill view's panel, remembering the list
// scroll position, and fetches live pool state unless it is cached or in flight.
func (m *model) openPoolDetail(view poolViewKind, row store.PoolRow) tea.Cmd {
	p, vp := m.pager(view), m.poolViewport(view)
	p.listYOffset = vp.YOffset
	p.expanded = &row
	m.syncPoolViewportHeights()
	if view == poolViewList {
		// The search box is hidden under the detail; it must not keep taking keystrokes.
		m.poolListSearch.Blur()
	}

	var cmd tea.Cmd
	if st, ok := m.poolDetails[row.PoolID]; !ok || (!st.loading && st.err != "") {
		m.poolDetails[row.PoolID] = &poolDetailState{loading: true}
		m.logInfo(fmt.Sprintf("Pool details: querying pool %s", shortPoolID(row.PoolID)))
		cmd = fetchPoolDetailsCmd(m.rpcURL, row.PoolID, int32(row.TickSpacing))
	}
	m.refreshPoolView(view)
	vp.GotoTop()
	return cmd
}

// syncPoolViewportHeights sets both pool viewports' heights to what renderUniswapPage
// will draw: the same m.h/2-4 panel height, minus the panel chrome unless a pool
// detail is open (the detail covers the whole panel).
func (m *model) syncPoolViewportHeights() {
	panelH := helpers.Max(1, m.h/2-4)
	m.v4EventsViewport.Height = uniswap.V4EventsViewportHeight(panelH, m.v4Pager.expanded != nil)
	m.poolListViewport.Height = uniswap.PoolListViewportHeight(panelH, m.poolListPager.expanded != nil)
}

// closePoolDetail returns view's panel to the list at the scroll position it was left at.
func (m *model) closePoolDetail(view poolViewKind) {
	p, vp := m.pager(view), m.poolViewport(view)
	p.expanded = nil
	m.syncPoolViewportHeights()
	m.refreshPoolView(view)
	vp.SetYOffset(p.listYOffset)
}

// handlePoolCardClick maps a click at content line/col within view's viewport: on the
// list, a card opens its full-panel detail; on the detail, the ✕ closes it. Returns
// handled=false when the click wasn't on a card.
func (m *model) handlePoolCardClick(view poolViewKind, line, col int) (tea.Cmd, bool) {
	p := m.pager(view)
	spans := m.v4EventsSpans
	if view == poolViewList {
		spans = m.poolListSpans
	}
	for _, sp := range spans {
		if line < sp.StartLine || line >= sp.EndLine {
			continue
		}
		if sp.Expanded {
			if line <= sp.CloseLine && col >= sp.CloseX1 && col < sp.CloseX2 {
				m.closePoolDetail(view)
			}
			return nil, true
		}
		for _, r := range p.rows {
			if r.PoolID == sp.PoolID {
				return m.openPoolDetail(view, r), true
			}
		}
		return nil, true
	}
	return nil, false
}
