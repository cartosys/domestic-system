package main

import (
	"context"
	"fmt"
	"image/color"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"
	// View packages - ready to use when delegating rendering
	"charm-wallet-tui/views/dapps"
	"charm-wallet-tui/views/details"
	"charm-wallet-tui/views/settings"
	"charm-wallet-tui/views/uniswap"
	"charm-wallet-tui/views/wallets"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/lucasb-eyer/go-colorful"
)

// -------------------- THEME (Lip Gloss) --------------------
// Styles now come from the styles package

var (
	cBg      = styles.CBg
	cPanel   = styles.CPanel
	cBorder  = styles.CBorder
	cMuted   = styles.CMuted
	cText    = styles.CText
	cAccent  = styles.CAccent
	cAccent2 = styles.CAccent2
	cWarn    = styles.CWarn

	appStyle       = styles.AppStyle
	titleStyle     = styles.TitleStyle
	panelStyle     = styles.PanelStyle
	navStyle       = styles.NavStyle
	hotkeyStyle    = lipgloss.NewStyle().Foreground(styles.CMuted)
	hotkeyKeyStyle = lipgloss.NewStyle().Foreground(styles.CAccent).Bold(true)
	helpRightStyle = lipgloss.NewStyle().Foreground(styles.CMuted)
)

// -------------------- DATA TYPES --------------------

type page int

// Temporary form field storage (package-level to avoid pointer-to-copy issues)
var (
	tempRPCFormName   string
	tempRPCFormURL    string
	tempNicknameField string
	tempDappName      string
	tempDappAddress   string
	tempDappIcon      string
	tempDappNetwork   string
	tempSendToAddr    string
	tempSendAmount    string
)

const (
	pageHome page = iota
	pageWallets
	pageDetails
	pageSettings
	pageDappBrowser
	pageUniswap
)

// clickableArea represents a clickable region on screen for addresses
type clickableArea struct {
	X, Y          int    // top-left position
	Width, Height int    // dimensions
	Address       string // wallet address to navigate to
}

type walletItem struct {
	addr string
}

func (w walletItem) Title() string       { return helpers.ShortenAddr(w.addr) }
func (w walletItem) Description() string { return w.addr }
func (w walletItem) FilterValue() string { return w.addr }

type tokenBalance struct {
	Symbol   string
	Decimals uint8
	Balance  *big.Int
}

type walletDetails struct {
	Address    string
	EthWei     *big.Int
	Tokens     []tokenBalance
	LoadedAt   time.Time
	ErrMessage string
}

// -------------------- MODEL --------------------

type model struct {
	w, h int

	activePage page

	// main list
	accounts       []config.WalletEntry
	selectedWallet int

	// add-wallet input
	adding          bool
	input           textinput.Model // address input
	nicknameInput   textinput.Model // nickname input
	focusedInput    int             // 0 = address, 1 = nickname
	addError        string          // error message when adding wallet (e.g., duplicate)
	addErrTime      time.Time       // time when error was shown
	ensLookupActive bool            // true if ENS lookup is in progress
	ensLookupAddr   string          // address being looked up

	// details state
	spin          spinner.Model
	loading       bool
	details       walletDetails
	detailsCache  map[string]walletDetails // cache wallet details by address
	rpcURL        string
	ethClient     *rpc.Client
	rpcConnected  bool // true if RPC is successfully connected
	rpcConnecting bool // true if connection attempt is in progress

	// token watchlist (simple starter set)
	// You can expand this (or load from config).
	tokenWatch []rpc.WatchedToken

	// clipboard feedback
	copiedMsg     string
	copiedMsgTime time.Time
	addressLineY  int // Y position of the address line in details view

	// transaction result panel copy feedback
	txCopiedMsg     string
	txCopiedMsgTime time.Time

	// settings state
	settingsMode   string // "list", "add", "edit"
	rpcURLs        []config.RPCUrl
	selectedRPCIdx int
	form           *huh.Form
	configPath     string

	// dApp browser state
	dapps           []config.DApp
	dappMode        string // "list", "add", "edit"
	selectedDappIdx int

	// home form
	homeForm *huh.Form

	// nickname editing
	nicknaming bool

	// currently highlighted address in wallet list
	highlightedAddress string
	// active address (the one marked with ★)
	activeAddress string

	// clickable areas for mouse support
	clickableAreas []clickableArea

	// logger panel
	logEnabled  bool
	logger      *log.Logger
	logBuffer   *strings.Builder
	logViewport viewport.Model
	logReady    bool
	logSpinner  spinner.Model

	// split view flag for wallets page
	detailsInWallets bool // when true, show details panel alongside wallet list

	// delete confirmation dialog
	showDeleteDialog        bool
	deleteDialogAddr        string
	deleteDialogIdx         int
	deleteDialogYesSelected bool // true = Yes button, false = No button
	showRPCDeleteDialog        bool
	deleteRPCDialogName        string
	deleteRPCDialogIdx         int
	deleteRPCDialogYesSelected bool

	// send button state
	sendButtonFocused bool
	showSendForm      bool
	sendForm          *huh.Form

	// transaction result panel
	showTxResultPanel bool
	txResultPackaging bool
	txResultHex       string
	txResultEIP681    string
	txResultError     string

	// Uniswap swap state
	uniswapFromTokenIdx    int    // index in available tokens
	uniswapToTokenIdx      int    // index in available tokens
	uniswapFromAmount      string // user input amount
	uniswapToAmount        string // estimated output amount
	uniswapFocusedField    int    // 0=from, 1=to, 2=swap button
	uniswapShowingSelector bool   // true when showing token selector popup
	uniswapSelectorFor     int    // 0=from, 1=to
	uniswapSelectorIdx     int    // selected index in token selector
	uniswapEstimating      bool   // true when estimating swap output
	uniswapQuote           *helpers.SwapQuote // current swap quote
	uniswapQuoteError      string             // error from quote fetch
	uniswapPriceImpactWarn string             // warning message for high price impact
	uniswapEditingFrom     bool               // true if user has been editing From field
	uniswapEditingTo       bool               // true if user has been editing To field
	// Track last quote parameters to avoid unnecessary fetches
	lastQuoteFromAmount   string // last amount used for forward quote
	lastQuoteToAmount     string // last amount used for reverse quote
	lastQuoteFromTokenIdx int    // last from token index used for quote
	lastQuoteToTokenIdx   int    // last to token index used for quote
}

// -------------------- INIT --------------------

func newModel() model {
	// config path
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".charm-wallet-config.json")

	// load config
	cfg := config.Load(configPath)

	// Load wallet entries from config
	accounts := cfg.Wallets
	if accounts == nil {
		accounts = []config.WalletEntry{}
	}

	// Find active wallet or default to first
	selectedIdx := 0
	activeAddr := ""
	for i, w := range accounts {
		if w.Active {
			selectedIdx = i
			activeAddr = w.Address
			break
		}
	}

	// input for address
	in := textinput.New()
	in.Placeholder = "Paste Public Address 0x…"
	in.Prompt = "Address: "
	in.PromptStyle = lipgloss.NewStyle().Foreground(cAccent)
	in.TextStyle = lipgloss.NewStyle().Foreground(cText)
	in.Cursor.Style = lipgloss.NewStyle().Foreground(cAccent2)
	in.CharLimit = 42
	in.Width = 48

	// input for nickname
	nicknameIn := textinput.New()
	nicknameIn.Placeholder = "Optional nickname"
	nicknameIn.Prompt = "Nickname: "
	nicknameIn.PromptStyle = lipgloss.NewStyle().Foreground(cAccent)
	nicknameIn.TextStyle = lipgloss.NewStyle().Foreground(cText)
	nicknameIn.Cursor.Style = lipgloss.NewStyle().Foreground(cAccent2)
	nicknameIn.CharLimit = 50
	nicknameIn.Width = 48

	// spinner
	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = lipgloss.NewStyle().Foreground(cAccent2)

	// rpc URL from environment
	rpcFromEnv := strings.TrimSpace(os.Getenv("ETH_RPC_URL"))

	// If no RPC in config but ENV is set, use ENV
	if len(cfg.RPCURLs) == 0 && rpcFromEnv != "" {
		cfg.RPCURLs = []config.RPCUrl{{Name: "Default", URL: rpcFromEnv, Active: true}}
	}

	// Find active RPC
	activeRPC := rpcFromEnv
	for _, r := range cfg.RPCURLs {
		if r.Active {
			activeRPC = r.URL
			break
		}
	}

	// starter token watchlist (Mainnet):
	// USDC, USDT, DAI, WETH
	watch := []rpc.WatchedToken{
		{Symbol: "WETH", Decimals: 18, Address: common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")},
		{Symbol: "USDC", Decimals: 6, Address: common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")},
		{Symbol: "USDT", Decimals: 6, Address: common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7")},
		{Symbol: "DAI", Decimals: 18, Address: common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F")},
	}

	// Initialize log viewport
	vp := viewport.New(0, 20) // Will be resized in Update on first WindowSizeMsg
	vp.Style = lipgloss.NewStyle().
		Foreground(cText).
		Background(cPanel)

	// Initialize log spinner
	logSpin := spinner.New()
	logSpin.Spinner = spinner.Dot
	logSpin.Style = lipgloss.NewStyle().Foreground(cAccent2)

	m := model{
		activePage:         pageWallets,
		accounts:           accounts,
		selectedWallet:     selectedIdx,
		highlightedAddress: activeAddr,
		activeAddress:      activeAddr,
		adding:             false,
		input:              in,
		nicknameInput:      nicknameIn,
		focusedInput:       0,
		spin:               sp,
		rpcURL:             activeRPC,
		tokenWatch:         watch,
		settingsMode:       "list",
		rpcURLs:            cfg.RPCURLs,
		selectedRPCIdx:     0,
		configPath:         configPath,
		logEnabled:         cfg.Logger,
		logViewport:        vp,
		logBuffer:          &strings.Builder{},
		logSpinner:         logSpin,
		detailsCache:       make(map[string]walletDetails),
		dapps:              cfg.Dapps,
		dappMode:           "list",
		selectedDappIdx:    0,
		detailsInWallets:   true, // Enable split panel view by default
	}

	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick}
	if m.logEnabled {
		cmds = append(cmds, initLogViewport(), m.logSpinner.Tick)
	}
	// connect if rpc is set
	if m.rpcURL != "" {
		m.rpcConnecting = true
		cmds = append(cmds, connectRPC(m.rpcURL))
	}
	return tea.Batch(cmds...)
}

// -------------------- COMMANDS / MESSAGES --------------------

type clipboardCopiedMsg struct{}

type txJsonCopiedMsg struct{}

type ensLookupResultMsg struct {
	address   string
	ensName   string
	err       error
	debugInfo string
}

type ensForwardResolveMsg struct {
	ensName     string // The .eth name that was resolved
	address     string // The resolved Ethereum address
	err         error
	debugInfo   string
}

type uniswapQuoteMsg struct {
	quote *helpers.SwapQuote
	err   error
}

type logInitMsg struct{}

type rpcConnectedMsg struct {
	client *rpc.Client
	err    error
}

func connectRPC(url string) tea.Cmd {
	return func() tea.Msg {
		result := rpc.Connect(url)
		return rpcConnectedMsg{client: result.Client, err: result.Error}
	}
}

func initLogViewport() tea.Cmd {
	return func() tea.Msg {
		return logInitMsg{}
	}
}

type detailsLoadedMsg struct {
	d   walletDetails
	err error
}

type packageTransactionMsg struct {
	pkg rpc.TransactionPackageEIP4527
	err error
}

func packageTransaction(fromAddr, toAddr string, ethAmount string, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		// Convert ETH amount to Wei
		amountFloat := new(big.Float)
		amountFloat.SetString(ethAmount)
		weiFloat := new(big.Float).Mul(amountFloat, big.NewFloat(1e18))
		amountWei, _ := weiFloat.Int(nil)

		// Call RPC package function using EIP-4527 format
		pkg, err := rpc.PackageTransactionEIP4527(
			common.HexToAddress(fromAddr),
			common.HexToAddress(toAddr),
			amountWei,
			rpcURL,
		)

		return packageTransactionMsg{pkg: pkg, err: err}
	}
}

func packageSwapTransaction(fromAddr string, fromToken, toToken uniswap.TokenOption, amountIn string, amountOutMin *big.Int, rpcURL string) tea.Cmd {
	return func() tea.Msg {
		// For Uniswap V2, we need to call the router contract
		// Router address on mainnet: 0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D
		routerAddr := "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"

		// Convert amount to token's base unit
		amountFloat := new(big.Float)
		amountFloat.SetString(amountIn)
		multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
		amountInUnits := new(big.Float).Mul(amountFloat, multiplier)
		amountInBig, _ := amountInUnits.Int(nil)

		// Build transaction description
		// For swaps, we create a transaction to the Uniswap router
		// The user needs to:
		// 1. Approve the token first (if not ETH)
		// 2. Execute the swap

		var txJSON string
		var eip681 string

		if fromToken.Symbol == "ETH" {
			// ETH -> Token swap: swapExactETHForTokens
			// For simplicity, create a transaction with the router address and value
			txJSON = fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x%s",
  "data": "SWAP: %s %s -> %s (min %s)",
  "note": "Uniswap V2 Swap: %s %s to %s %s"
}`,
				fromAddr,
				routerAddr,
				amountInBig.Text(16),
				amountIn, fromToken.Symbol,
				toToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				amountIn, fromToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				toToken.Symbol,
			)
			eip681 = fmt.Sprintf("%s?value=%s", routerAddr, amountInBig.String())
		} else if toToken.Symbol == "ETH" {
			// Token -> ETH swap: swapExactTokensForETH
			txJSON = fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x0",
  "data": "SWAP: %s %s -> %s (min %s)",
  "note": "Uniswap V2 Swap: %s %s to %s %s. IMPORTANT: Approve token first!"
}`,
				fromAddr,
				routerAddr,
				amountIn, fromToken.Symbol,
				toToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				amountIn, fromToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				toToken.Symbol,
			)
			eip681 = routerAddr
		} else {
			// Token -> Token swap: swapExactTokensForTokens
			txJSON = fmt.Sprintf(`{
  "from": "%s",
  "to": "%s",
  "value": "0x0",
  "data": "SWAP: %s %s -> %s (min %s)",
  "note": "Uniswap V2 Swap: %s %s to %s %s. IMPORTANT: Approve token first!"
}`,
				fromAddr,
				routerAddr,
				amountIn, fromToken.Symbol,
				toToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				amountIn, fromToken.Symbol,
				new(big.Float).Quo(new(big.Float).SetInt(amountOutMin), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))).Text('f', 6),
				toToken.Symbol,
			)
			eip681 = routerAddr
		}

		pkg := rpc.TransactionPackageEIP4527{
			JSON:   txJSON,
			QRData: eip681,
		}

		return packageTransactionMsg{pkg: pkg, err: nil}
	}
}

func loadDetails(client *rpc.Client, addr common.Address, watch []rpc.WatchedToken) tea.Cmd {
	return func() tea.Msg {
		rpcDetails := rpc.LoadWalletDetails(client, addr, watch)

		// Convert rpc.WalletDetails to our walletDetails type
		d := walletDetails{
			Address:    rpcDetails.Address,
			EthWei:     rpcDetails.EthWei,
			LoadedAt:   rpcDetails.LoadedAt,
			ErrMessage: rpcDetails.ErrMessage,
		}

		// Convert token balances
		for _, t := range rpcDetails.Tokens {
			d.Tokens = append(d.Tokens, tokenBalance{
				Symbol:   t.Symbol,
				Decimals: t.Decimals,
				Balance:  t.Balance,
			})
		}

		return detailsLoadedMsg{d: d, err: nil}
	}
}

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(text)
		if err == nil {
			return clipboardCopiedMsg{}
		}
		return nil
	}
}

func copyTxJsonToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(text)
		if err == nil {
			return txJsonCopiedMsg{}
		}
		return nil
	}
}

func clearClipboardMsg() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return struct{ clearClipboard bool }{true}
	})
}

func lookupENS(client *rpc.Client, address string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ensLookupResultMsg{address: address, ensName: "", err: fmt.Errorf("no RPC client"), debugInfo: ""}
		}
		
		// Perform ENS reverse lookup
		result := helpers.LookupENS(address, client.URL)
		return ensLookupResultMsg{
			address:   address,
			ensName:   result.Name,
			err:       result.Error,
			debugInfo: result.DebugInfo,
		}
	}
}

func resolveENS(client *rpc.Client, ensName string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return ensForwardResolveMsg{ensName: ensName, address: "", err: fmt.Errorf("no RPC client"), debugInfo: ""}
		}
		
		// Perform ENS forward resolution
		result := helpers.ResolveENS(ensName, client.URL)
		return ensForwardResolveMsg{
			ensName:   ensName,
			address:   result.Name, // Name field contains the resolved address
			err:       result.Error,
			debugInfo: result.DebugInfo,
		}
	}
}

func fetchUniswapQuote(client *rpc.Client, pairAddr, tokenInAddr common.Address, amountIn *big.Int) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}

		quote, err := helpers.GetSwapQuote(client.Client, pairAddr, tokenInAddr, amountIn)
		return uniswapQuoteMsg{quote, err}
	}
}

func fetchReverseUniswapQuote(client *rpc.Client, pairAddr, tokenInAddr common.Address, amountOut *big.Int, fromTokenDecimals uint8) tea.Cmd {
	return func() tea.Msg {
		if client == nil || client.Client == nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("no RPC client")}
		}

		ctx := context.Background()

		// Get the pair info
		pair, err := helpers.GetUniswapV2Pair(client.Client, pairAddr)
		if err != nil {
			return uniswapQuoteMsg{nil, err}
		}

		// Get reserves using same method as GetSwapQuote
		getReservesSelector := []byte{0x09, 0x02, 0xf1, 0xac}
		reservesMsg := ethereum.CallMsg{
			To:   &pairAddr,
			Data: getReservesSelector,
		}
		reservesData, err := client.Client.CallContract(ctx, reservesMsg, nil)
		if err != nil {
			return uniswapQuoteMsg{nil, fmt.Errorf("failed to get reserves: %w", err)}
		}

		// Parse reserves
		if len(reservesData) < 32 {
			return uniswapQuoteMsg{nil, fmt.Errorf("invalid reserves data length: %d", len(reservesData))}
		}

		reserve0 := new(big.Int).SetBytes(reservesData[0:32])
		reserve1 := big.NewInt(0)
		if len(reservesData) >= 64 {
			reserve1 = new(big.Int).SetBytes(reservesData[32:64])
		}

		// Determine which reserve is which based on token order
		var reserveIn, reserveOut *big.Int
		if tokenInAddr == pair.Token0 {
			reserveIn = reserve0
			reserveOut = reserve1
		} else {
			reserveIn = reserve1
			reserveOut = reserve0
		}

		// Calculate required input amount using reverse formula:
		// amountIn = (reserveIn * amountOut * 1000) / ((reserveOut - amountOut) * 997) + 1
		numerator := new(big.Int).Mul(reserveIn, amountOut)
		numerator = new(big.Int).Mul(numerator, big.NewInt(1000))

		denominator := new(big.Int).Sub(reserveOut, amountOut)
		denominator = new(big.Int).Mul(denominator, big.NewInt(997))

		if denominator.Sign() <= 0 {
			return uniswapQuoteMsg{nil, fmt.Errorf("insufficient liquidity for desired output amount")}
		}

		amountIn := new(big.Int).Div(numerator, denominator)
		amountIn = new(big.Int).Add(amountIn, big.NewInt(1)) // Add 1 for rounding

		// Calculate effective price
		effectivePrice := 0.0
		if amountIn.Sign() > 0 {
			amountInFloat := new(big.Float).SetInt(amountIn)
			amountOutFloat := new(big.Float).SetInt(amountOut)
			priceFloat := new(big.Float).Quo(amountOutFloat, amountInFloat)
			effectivePrice, _ = priceFloat.Float64()
		}

		// Calculate price impact
		priceImpact := 0.0
		if reserveIn.Sign() > 0 && reserveOut.Sign() > 0 {
			spotPrice := new(big.Float).Quo(new(big.Float).SetInt(reserveOut), new(big.Float).SetInt(reserveIn))
			spotPriceFloat, _ := spotPrice.Float64()
			
			if spotPriceFloat > 0 {
				priceImpact = ((spotPriceFloat - effectivePrice) / spotPriceFloat) * 100
			}
		}

		quote := &helpers.SwapQuote{
			AmountIn:       amountIn,
			AmountOut:      amountOut,
			Token0Reserve:  reserve0,
			Token1Reserve:  reserve1,
			PriceImpact:    priceImpact,
			EffectivePrice: effectivePrice,
		}

		return uniswapQuoteMsg{quote, nil}
	}
}

// -------------------- LOG FUNCTIONS --------------------

// addLog adds a log entry with timestamp and type
func (m *model) addLog(logType, message string) {
	if !m.logEnabled || !m.logReady || m.logger == nil {
		return
	}

	// Use the logger to write messages
	switch logType {
	case "info":
		m.logger.Info(message)
	case "success":
		m.logger.Info("✓", "msg", message)
	case "error":
		m.logger.Error(message)
	case "warning":
		m.logger.Warn(message)
	case "debug":
		m.logger.Debug(message)
	default:
		m.logger.Print(message)
	}

	// Update viewport content
	m.updateLogViewport()
}

// loadSelectedWalletDetails loads details for the currently selected wallet if split view is enabled
func (m *model) loadSelectedWalletDetails() tea.Cmd {
	if !m.detailsInWallets || len(m.accounts) == 0 {
		return nil
	}

	addr := m.accounts[m.selectedWallet].Address
	// Check if we have cached details
	cachedDetails, hasCached := m.detailsCache[strings.ToLower(addr)]
	if hasCached {
		m.details = cachedDetails
		m.loading = false
		return nil
	}

	// Load fresh details
	m.loading = true
	m.details = walletDetails{Address: addr}
	ethAddr := common.HexToAddress(addr)
	return loadDetails(m.ethClient, ethAddr, m.tokenWatch)
}

// updateLogViewport refreshes the viewport content with log output
func (m *model) updateLogViewport() {
	if !m.logReady || m.logBuffer == nil {
		return
	}

	// Get content from log buffer
	content := m.logBuffer.String()
	m.logViewport.SetContent(content)
	// Scroll to bottom to show latest entries
	m.logViewport.GotoBottom()
}

func (m model) textInputActive() bool {
	if m.adding {
		return true
	}
	if m.showSendForm && m.sendForm != nil {
		return true
	}
	if m.nicknaming && m.form != nil {
		return true
	}
	if (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
		return true
	}
	if (m.dappMode == "add" || m.dappMode == "edit") && m.form != nil {
		return true
	}
	return false
}

// buildTokenList builds a list of available tokens from wallet details for Uniswap
func (m model) buildTokenList() []uniswap.TokenOption {
	var tokens []uniswap.TokenOption
	
	// Add ETH first
	tokens = append(tokens, uniswap.TokenOption{
		Symbol:   "ETH",
		Balance:  m.details.EthWei,
		Decimals: 18,
		IsETH:    true,
	})
	
	// Add tokens from watchlist that have balances
	for _, token := range m.details.Tokens {
		tokens = append(tokens, uniswap.TokenOption{
			Symbol:   token.Symbol,
			Balance:  token.Balance,
			Decimals: token.Decimals,
			IsETH:    false,
		})
	}
	
	return tokens
}

// maybeRequestUniswapQuote triggers a swap quote fetch if conditions are met
func (m *model) maybeRequestUniswapQuote() tea.Cmd {
	// Check if we have valid inputs
	if m.uniswapFromAmount == "" || m.uniswapFromAmount == "0" {
		// Clear previous quote state when amount is cleared
		m.uniswapToAmount = ""
		m.uniswapQuote = nil
		m.uniswapQuoteError = ""
		m.uniswapPriceImpactWarn = ""
		m.lastQuoteFromAmount = ""
		return nil
	}

	tokens := m.buildTokenList()
	if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
		return nil
	}
	if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
		return nil
	}

	fromToken := tokens[m.uniswapFromTokenIdx]
	toToken := tokens[m.uniswapToTokenIdx]

	// Can't swap same token
	if fromToken.Symbol == toToken.Symbol {
		return nil
	}

	// Check if anything has changed since last quote
	if m.lastQuoteFromAmount == m.uniswapFromAmount &&
		m.lastQuoteFromTokenIdx == m.uniswapFromTokenIdx &&
		m.lastQuoteToTokenIdx == m.uniswapToTokenIdx &&
		m.uniswapQuote != nil &&
		m.uniswapToAmount != "" {
		// Nothing changed and we already have a quote, no need to fetch again
		return nil
	}

	// Parse amount
	amountFloat := new(big.Float)
	_, ok := amountFloat.SetString(m.uniswapFromAmount)
	if !ok {
		return nil
	}

	// Convert to token's base unit (e.g., wei for ETH, smallest unit for tokens)
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
	amountInTokenUnits := new(big.Float).Mul(amountFloat, multiplier)
	amountIn, _ := amountInTokenUnits.Int(nil)

	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil
	}

	// Determine pair address and token addresses
	// For now, only support USDC <-> WETH swaps
	var pairAddr, tokenInAddr common.Address
	
	// Map token symbols to addresses
	if (fromToken.Symbol == "USDC" && toToken.Symbol == "ETH") || (fromToken.Symbol == "ETH" && toToken.Symbol == "USDC") {
		pairAddr = helpers.USDCWETHPairAddress
		if fromToken.Symbol == "USDC" {
			tokenInAddr = helpers.USDCAddress
		} else {
			tokenInAddr = helpers.WETHAddress
		}
	} else {
		// Unsupported pair
		m.addLog("warn", fmt.Sprintf("Swap pair %s/%s not supported yet", fromToken.Symbol, toToken.Symbol))
		return nil
	}

	// Update tracking state before fetching
	m.lastQuoteFromAmount = m.uniswapFromAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx

	// Clear previous quote state when fetching new quote
	m.uniswapToAmount = ""
	m.uniswapQuote = nil
	m.uniswapQuoteError = ""
	m.uniswapPriceImpactWarn = ""

	m.uniswapEstimating = true
	return fetchUniswapQuote(m.ethClient, pairAddr, tokenInAddr, amountIn)
}

// maybeRequestReverseUniswapQuote triggers a reverse swap quote fetch (calculate From from To amount)
func (m *model) maybeRequestReverseUniswapQuote() tea.Cmd {
	// Check if we have valid inputs
	if m.uniswapToAmount == "" || m.uniswapToAmount == "0" {
		// Clear previous quote state when amount is cleared
		m.uniswapFromAmount = ""
		m.uniswapQuote = nil
		m.uniswapQuoteError = ""
		m.uniswapPriceImpactWarn = ""
		return nil
	}

	tokens := m.buildTokenList()
	if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
		return nil
	}
	if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
		return nil
	}

	fromToken := tokens[m.uniswapFromTokenIdx]
	toToken := tokens[m.uniswapToTokenIdx]

	// Can't swap same token
	if fromToken.Symbol == toToken.Symbol {
		return nil
	}

	// Check if anything has changed since last reverse quote
	if m.lastQuoteToAmount == m.uniswapToAmount &&
		m.lastQuoteFromTokenIdx == m.uniswapFromTokenIdx &&
		m.lastQuoteToTokenIdx == m.uniswapToTokenIdx &&
		m.uniswapQuote != nil &&
		m.uniswapFromAmount != "" {
		// Nothing changed and we already have a quote, no need to fetch again
		return nil
	}

	// Parse desired output amount
	amountOutFloat := new(big.Float)
	_, ok := amountOutFloat.SetString(m.uniswapToAmount)
	if !ok {
		return nil
	}

	// Convert to token's base unit
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))
	amountOutTokenUnits := new(big.Float).Mul(amountOutFloat, multiplier)
	amountOut, _ := amountOutTokenUnits.Int(nil)

	if amountOut == nil || amountOut.Sign() <= 0 {
		return nil
	}

	// Determine pair address and token addresses
	var pairAddr, tokenInAddr common.Address
	
	if (fromToken.Symbol == "USDC" && toToken.Symbol == "ETH") || (fromToken.Symbol == "ETH" && toToken.Symbol == "USDC") {
		pairAddr = helpers.USDCWETHPairAddress
		if fromToken.Symbol == "USDC" {
			tokenInAddr = helpers.USDCAddress
		} else {
			tokenInAddr = helpers.WETHAddress
		}
	} else {
		// Unsupported pair
		m.addLog("warn", fmt.Sprintf("Swap pair %s/%s not supported yet", fromToken.Symbol, toToken.Symbol))
		return nil
	}

	// Update tracking state before fetching
	m.lastQuoteToAmount = m.uniswapToAmount
	m.lastQuoteFromTokenIdx = m.uniswapFromTokenIdx
	m.lastQuoteToTokenIdx = m.uniswapToTokenIdx

	// Clear previous from amount and quote state
	m.uniswapFromAmount = ""
	m.uniswapQuote = nil
	m.uniswapQuoteError = ""
	m.uniswapPriceImpactWarn = ""
	m.uniswapEstimating = true

	m.addLog("info", fmt.Sprintf("Calculating required input for %s %s", m.uniswapToAmount, toToken.Symbol))

	return fetchReverseUniswapQuote(m.ethClient, pairAddr, tokenInAddr, amountOut, fromToken.Decimals)
}

func (m *model) createSendForm() {
	tempSendToAddr = ""
	tempSendAmount = ""

	m.sendForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Send To").
				Description("Enter a valid Ethereum address (Ctrl+v to paste)").
				Value(&tempSendToAddr).
				Placeholder("0x...").
				Validate(func(s string) error {
					if !helpers.IsValidEthAddress(s) {
						return fmt.Errorf("invalid ethereum address")
					}
					return nil
				}),

			huh.NewInput().
				Title("Amount (ETH)").
				Description(fmt.Sprintf("Available: %s ETH", helpers.FormatETH(m.details.EthWei))).
				Value(&tempSendAmount).
				Placeholder("0.0").
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("amount is required")
					}
					// Parse amount as big.Float
					amount := new(big.Float)
					_, ok := amount.SetString(s)
					if !ok {
						return fmt.Errorf("invalid amount")
					}
					// Check if amount is <= balance
					balanceFloat := new(big.Float).SetInt(m.details.EthWei)
					balanceETH := new(big.Float).Quo(balanceFloat, big.NewFloat(1e18))
					if amount.Cmp(balanceETH) > 0 {
						return fmt.Errorf("amount exceeds balance")
					}
					if amount.Cmp(big.NewFloat(0)) <= 0 {
						return fmt.Errorf("amount must be greater than 0")
					}
					return nil
				}),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.sendForm.Init()
}

func (m *model) createAddRPCForm() {
	tempRPCFormName = ""
	tempRPCFormURL = ""

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("RPC Name").
				Description("A friendly name for this RPC endpoint").
				Value(&tempRPCFormName).
				Placeholder("My Infura Node"),

			huh.NewInput().
				Title("RPC URL").
				Description("The complete RPC URL (https://...)").
				Value(&tempRPCFormURL).
				Placeholder("https://mainnet.infura.io/v3/..."),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) createEditRPCForm(idx int) {
	if idx < 0 || idx >= len(m.rpcURLs) {
		return
	}

	rpc := m.rpcURLs[idx]
	tempRPCFormName = rpc.Name
	tempRPCFormURL = rpc.URL

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("RPC Name").
				Value(&tempRPCFormName).
				Placeholder("My Node"),

			huh.NewInput().
				Title("RPC URL").
				Value(&tempRPCFormURL).
				Placeholder("https://..."),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) createNicknameForm() {
	// Find current wallet's nickname
	tempNicknameField = ""
	for _, w := range m.accounts {
		if strings.EqualFold(w.Address, m.details.Address) {
			tempNicknameField = w.Name
			break
		}
	}

	placeholderText := "Enter nickname"
	if tempNicknameField != "" {
		placeholderText = tempNicknameField
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Wallet Nickname").
				Description("Set a friendly name for this wallet").
				Value(&tempNicknameField).
				Placeholder(placeholderText),
		),
	).WithTheme(huh.ThemeCatppuccin())

	// Initialize the form
	m.form.Init()
}

func (m *model) createAddDappForm() {
	tempDappName = ""
	tempDappAddress = ""
	tempDappIcon = ""
	tempDappNetwork = ""

	// Build network options from RPC URLs
	networkOptions := []huh.Option[string]{}
	for _, rpcURL := range m.rpcURLs {
		networkOptions = append(networkOptions, huh.NewOption(rpcURL.Name, rpcURL.Name))
	}

	// Find the active RPC URL name as default
	defaultNetwork := ""
	for _, rpcURL := range m.rpcURLs {
		if rpcURL.Active {
			defaultNetwork = rpcURL.Name
			break
		}
	}
	tempDappNetwork = defaultNetwork

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("dApp Name").
				Description("A friendly name for this dApp").
				Value(&tempDappName).
				Placeholder("Uniswap"),

			huh.NewInput().
				Title("dApp Address").
				Description("The URL or address of the dApp").
				Value(&tempDappAddress).
				Placeholder("https://app.uniswap.org"),

			huh.NewInput().
				Title("Icon").
				Description("Icon or emoji for the dApp (optional)").
				Value(&tempDappIcon).
				Placeholder("🦄"),

			huh.NewSelect[string]().
				Options(networkOptions...).
				Title("Network").
				Description("Choose the network for this dApp").
				Value(&tempDappNetwork),
		),
	).WithTheme(huh.ThemeCatppuccin())

	m.form.Init()
}

func (m *model) createEditDappForm(idx int) {
	if idx < 0 || idx >= len(m.dapps) {
		return
	}

	dapp := m.dapps[idx]
	tempDappName = dapp.Name
	tempDappAddress = dapp.Address
	tempDappIcon = dapp.Icon
	tempDappNetwork = dapp.Network

	// Build network options from RPC URLs
	networkOptions := []huh.Option[string]{}
	for _, rpcURL := range m.rpcURLs {
		networkOptions = append(networkOptions, huh.NewOption(rpcURL.Name, rpcURL.Name))
	}

	// If current network is empty, use active RPC URL as default
	if tempDappNetwork == "" {
		for _, rpcURL := range m.rpcURLs {
			if rpcURL.Active {
				tempDappNetwork = rpcURL.Name
				break
			}
		}
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("dApp Name").
				Value(&tempDappName).
				Placeholder("Uniswap"),

			huh.NewInput().
				Title("dApp Address").
				Value(&tempDappAddress).
				Placeholder("https://app.uniswap.org"),

			huh.NewInput().
				Title("Icon").
				Value(&tempDappIcon).
				Placeholder("🦄"),

			huh.NewSelect[string]().
				Options(networkOptions...).
				Title("Network").
				Description("Choose the network for this dApp").
				Value(&tempDappNetwork),
		),
	).WithTheme(huh.ThemeCatppuccin())

	m.form.Init()
}

// -------------------- UPDATE --------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle send form updates first
	if m.activePage == pageWallets && m.showSendForm && m.sendForm != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.showSendForm = false
			m.sendForm = nil
			return m, nil
		}

		form, cmd := m.sendForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.sendForm = f

			// Check if form is completed
			if m.sendForm.State == huh.StateCompleted {
				// Package the transaction
				m.addLog("info", fmt.Sprintf("Packaging transaction: %s ETH to %s", tempSendAmount, helpers.ShortenAddr(tempSendToAddr)))
				m.showSendForm = false
				m.sendForm = nil
				m.showTxResultPanel = true
				m.txResultPackaging = true
				m.txResultHex = ""
				m.txResultError = ""
				return m, packageTransaction(m.activeAddress, tempSendToAddr, tempSendAmount, m.rpcURL)
			}

			// Check if form was aborted (ESC pressed)
			if m.sendForm.State == huh.StateAborted {
				m.showSendForm = false
				m.sendForm = nil
				return m, nil
			}
		}
		return m, cmd
	}

	if m.activePage == pageHome {
		// TODO: home view not implemented yet
		// Temporarily disabled until home view is created
		m.activePage = pageWallets
		return m, m.loadSelectedWalletDetails()
	}

	// Handle form updates first (before message switching)
	if m.activePage == pageWallets && m.showSendForm && m.sendForm != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.showSendForm = false
			m.sendForm = nil
			return m, nil
		}

		form, cmd := m.sendForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.sendForm = f

			// Check if form is completed
			if m.sendForm.State == huh.StateCompleted {
				// Package the transaction
				m.addLog("info", fmt.Sprintf("Packaging transaction: %s ETH to %s", tempSendAmount, helpers.ShortenAddr(tempSendToAddr)))
				m.showSendForm = false
				m.sendForm = nil
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.sendForm.State == huh.StateAborted {
				m.showSendForm = false
				m.sendForm = nil
				return m, nil
			}
		}
		return m, cmd
	}

	// Handle form updates first (before message switching)
	if m.activePage == pageDetails && m.nicknaming && m.form != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.nicknaming = false
			m.form = nil
			return m, nil
		}

		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f

			// Check if form is completed
			if m.form.State == huh.StateCompleted {
				// Save nickname to wallet entry
				for i := range m.accounts {
					if strings.EqualFold(m.accounts[i].Address, m.details.Address) {
						oldName := m.accounts[i].Name
						m.accounts[i].Name = strings.TrimSpace(tempNicknameField)
						if oldName == "" && m.accounts[i].Name != "" {
							m.addLog("success", fmt.Sprintf("Set nickname `%s` for wallet `%s`", m.accounts[i].Name, helpers.ShortenAddr(m.details.Address)))
						} else if m.accounts[i].Name == "" {
							m.addLog("info", fmt.Sprintf("Cleared nickname for wallet `%s`", helpers.ShortenAddr(m.details.Address)))
						} else {
							m.addLog("success", fmt.Sprintf("Updated nickname to `%s` for wallet `%s`", m.accounts[i].Name, helpers.ShortenAddr(m.details.Address)))
						}
						break
					}
				}
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
				m.nicknaming = false
				m.form = nil
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.form.State == huh.StateAborted {
				m.nicknaming = false
				m.form = nil
				return m, nil
			}
		}
		return m, cmd
	}

	if (m.activePage == pageDetails || m.activePage == pageDappBrowser) && (m.dappMode == "add" || m.dappMode == "edit") && m.form != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.dappMode = "list"
			m.form = nil
			return m, nil
		}

		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f

			// Check if form is completed
			if m.form.State == huh.StateCompleted {
				if m.dappMode == "add" {
					if tempDappName != "" && tempDappAddress != "" {
						newDapp := config.DApp{Name: tempDappName, Address: tempDappAddress, Icon: tempDappIcon, Network: tempDappNetwork}
						m.dapps = append(m.dapps, newDapp)
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Added dApp: `%s`", tempDappName))
					}
				} else if m.dappMode == "edit" {
					if m.selectedDappIdx >= 0 && m.selectedDappIdx < len(m.dapps) {
						m.dapps[m.selectedDappIdx].Name = tempDappName
						m.dapps[m.selectedDappIdx].Address = tempDappAddress
						m.dapps[m.selectedDappIdx].Icon = tempDappIcon
						m.dapps[m.selectedDappIdx].Network = tempDappNetwork
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Updated dApp: `%s`", tempDappName))
					}
				}
				m.dappMode = "list"
				m.form = nil
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.form.State == huh.StateAborted {
				m.dappMode = "list"
				m.form = nil
				return m, nil
			}
		}
		return m, cmd
	}

	if m.activePage == pageSettings && (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
		// Intercept ESC key to cancel form
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
			m.settingsMode = "list"
			m.form = nil
			return m, nil
		}

		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f

			// Check if form is completed
			if m.form.State == huh.StateCompleted {
				if m.settingsMode == "add" {
					if tempRPCFormName != "" && tempRPCFormURL != "" {
						newRPC := config.RPCUrl{Name: tempRPCFormName, URL: tempRPCFormURL, Active: false}
						m.rpcURLs = append(m.rpcURLs, newRPC)
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Added RPC endpoint: `%s` (%s)", tempRPCFormName, tempRPCFormURL))
					}
				} else if m.settingsMode == "edit" {
					if m.selectedRPCIdx >= 0 && m.selectedRPCIdx < len(m.rpcURLs) {
						m.rpcURLs[m.selectedRPCIdx].Name = tempRPCFormName
						m.rpcURLs[m.selectedRPCIdx].URL = tempRPCFormURL
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("success", fmt.Sprintf("Updated RPC endpoint: `%s`", tempRPCFormName))
					}
				}
				m.settingsMode = "list"
				m.form = nil
				// Return without the form's cmd to ensure we're back in list mode
				return m, nil
			}

			// Check if form was aborted (ESC pressed)
			if m.form.State == huh.StateAborted {
				m.settingsMode = "list"
				m.form = nil
				return m, nil
			}
		}
		return m, cmd
	}

	switch msg := msg.(type) {

	case logInitMsg:
		if !m.logEnabled {
			return m, nil
		}
		// Create logger that writes to our buffer
		m.logger = log.NewWithOptions(m.logBuffer, log.Options{
			ReportTimestamp: true,
			TimeFormat:      "15:04:05",
			Prefix:          "",
		})
		// Set log level and styling
		m.logger.SetLevel(log.DebugLevel)
		m.logger.SetStyles(&log.Styles{
			Timestamp: lipgloss.NewStyle().Foreground(cMuted),
			Caller:    lipgloss.NewStyle().Faint(true),
			Prefix:    lipgloss.NewStyle().Bold(true).Foreground(cAccent2),
			Message:   lipgloss.NewStyle().Foreground(cText),
			Key:       lipgloss.NewStyle().Foreground(cAccent),
			Value:     lipgloss.NewStyle().Foreground(cText),
			Separator: lipgloss.NewStyle().Faint(true),
			Levels: map[log.Level]lipgloss.Style{
				log.DebugLevel: lipgloss.NewStyle().Foreground(cMuted).SetString("DEBUG"),
				log.InfoLevel:  lipgloss.NewStyle().Foreground(cAccent2).SetString("INFO"),
				log.WarnLevel:  lipgloss.NewStyle().Foreground(cWarn).SetString("WARN"),
				log.ErrorLevel: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).SetString("ERROR"),
			},
		})
		m.logReady = true
		m.addLog("info", "Logger enabled")
		return m, nil

	case rpcConnectedMsg:
		m.rpcConnecting = false
		if msg.err != nil {
			// Connection failed
			m.ethClient = nil
			m.rpcConnected = false
			m.addLog("error", fmt.Sprintf("RPC connection failed: `%s`", msg.err.Error()))
		} else {
			// Connection successful
			m.ethClient = msg.client
			m.rpcConnected = true
			m.addLog("success", fmt.Sprintf("RPC connected to `%s`", msg.client.URL))
			// Load active account details automatically when on wallet page with split view
			if m.activePage == pageWallets && m.detailsInWallets && len(m.accounts) > 0 {
				return m, m.loadSelectedWalletDetails()
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height

		// Only initialize viewport if log is enabled
		if m.logEnabled {
			// Update log viewport dimensions
			// Width accounts for border and padding
			m.logViewport.Width = max(0, msg.Width-6)
			// Height will be calculated dynamically in renderLogPanel
			if m.logReady {
				m.updateLogViewport()
			}
		}

		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		var cmds []tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)
		// Update log spinner too if log is enabled but not ready
		if m.logEnabled && !m.logReady {
			m.logSpinner, cmd = m.logSpinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case detailsLoadedMsg:
		m.loading = false
		m.details = msg.d
		// Cache the loaded details
		if m.details.Address != "" {
			m.detailsCache[strings.ToLower(m.details.Address)] = m.details
		}
		if msg.err != nil && m.details.ErrMessage == "" {
			m.details.ErrMessage = "Failed to load wallet details."
			m.addLog("error", fmt.Sprintf("Failed to load details for `%s`", helpers.ShortenAddr(m.details.Address)))
		} else if m.details.ErrMessage != "" {
			m.addLog("error", fmt.Sprintf("Wallet `%s`: %s", helpers.ShortenAddr(m.details.Address), m.details.ErrMessage))
		} else {
			m.addLog("success", fmt.Sprintf("Loaded details for `%s` - ETH: %s", helpers.ShortenAddr(m.details.Address), helpers.FormatETH(m.details.EthWei)))
		}
		return m, nil

	case packageTransactionMsg:
		m.txResultPackaging = false
		if msg.err != nil {
			m.txResultError = msg.err.Error()
			m.addLog("error", "Transaction packaging failed: "+msg.err.Error())
		} else {
			m.txResultHex = msg.pkg.JSON
			m.txResultEIP681 = msg.pkg.QRData
			m.addLog("success", "Transaction packaged successfully (EIP-4527)")
		}
		return m, nil

	case tea.KeyMsg:
		// Handle transaction result panel FIRST (before any other keys)
		if m.showTxResultPanel {
			switch msg.String() {
			case "ctrl+c":
				// Copy transaction JSON to clipboard
				if m.txResultHex != "" {
					m.addLog("info", "Copied transaction JSON to clipboard")
					return m, copyTxJsonToClipboard(m.txResultHex)
				}
				return m, nil
			case "esc", "enter":
				m.showTxResultPanel = false
				m.txResultHex = ""
				m.txResultEIP681 = ""
				m.txResultError = ""
				m.txResultPackaging = false
				m.txCopiedMsg = ""
				return m, nil
			}
			return m, nil
		}

		allowMenuHotkeys := !m.textInputActive()
		// global keys
		if allowMenuHotkeys {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit

			case "l", "L":
				// Toggle logger
				m.logEnabled = !m.logEnabled
				if m.logEnabled {
					// Initialize viewport when enabling
					if m.w > 0 {
						m.logViewport.Width = m.w - 6
					}
					m.logReady = false
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
					return m, tea.Batch(initLogViewport(), m.logSpinner.Tick)
				}
				// Clear logs and de-initialize when disabling
				if m.logBuffer != nil {
					m.logBuffer.Reset()
				}
				m.logger = nil
				m.logReady = false
				config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
				return m, nil

			case "pageup", "pagedown":
				// Allow scrolling in log viewport when enabled
				if m.logEnabled && m.logReady {
					var cmd tea.Cmd
					m.logViewport, cmd = m.logViewport.Update(msg)
					return m, cmd
				}
			}
		}

		// page-specific behavior
		switch m.activePage {

		case pageHome:
			// Home page - form handles its own keys
			// No additional key handling needed
			return m, nil

		case pageWallets:
			// Handle delete confirmation dialog
			if m.showDeleteDialog {
				switch msg.String() {
				case "left", "right", "tab":
					// Toggle between Yes and No buttons
					m.deleteDialogYesSelected = !m.deleteDialogYesSelected
					return m, nil
				case "enter":
					// Execute based on selected button
					if m.deleteDialogYesSelected {
						// Confirm deletion (Yes button)
						idx := m.deleteDialogIdx
						deletedAddr := m.deleteDialogAddr
						m.accounts = append(m.accounts[:idx], m.accounts[idx+1:]...)
						// Update selected index
						if m.selectedWallet >= len(m.accounts) && m.selectedWallet > 0 {
							m.selectedWallet--
						}
						// Update highlighted address and check if active was deleted
						if len(m.accounts) > 0 {
							m.highlightedAddress = m.accounts[m.selectedWallet].Address
							// Update active address if needed
							m.activeAddress = ""
							for _, w := range m.accounts {
								if w.Active {
									m.activeAddress = w.Address
									break
								}
							}
						} else {
							m.highlightedAddress = ""
							m.activeAddress = ""
						}
						// Save wallets to config
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						m.addLog("warning", fmt.Sprintf("Deleted wallet `%s`", helpers.ShortenAddr(deletedAddr)))
						m.showDeleteDialog = false
						// Load details for the newly selected wallet if split view is enabled
						return m, m.loadSelectedWalletDetails()
					} else {
						// Cancel deletion (No button)
						m.showDeleteDialog = false
						return m, nil
					}
				case "esc":
					// Cancel deletion
					m.showDeleteDialog = false
					return m, nil
				}
				return m, nil
			}

			// Handle send button focus toggle with Tab

			switch msg.String() {
			case "tab":
				// Don't allow tab navigation when send form or add wallet form is active
				if m.showSendForm || m.adding {

				} else {
					// Only allow focusing send button if ETH balance > 0
					if m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
						m.sendButtonFocused = !m.sendButtonFocused
					}
					return m, nil
				}

			case "enter":
				// Show send form when send button is focused
				if m.sendButtonFocused && m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
					m.createSendForm()
					m.showSendForm = true
					m.sendButtonFocused = false
					return m, nil
				}
			}
			// adding flow
			if m.adding {
				switch msg.String() {
				case "esc", "escape":
					// Cancel adding mode
					m.adding = false
					m.input.SetValue("")
					m.nicknameInput.SetValue("")
					m.input.Blur()
					m.nicknameInput.Blur()
					m.focusedInput = 0
					m.addError = ""
					m.ensLookupActive = false
					m.ensLookupAddr = ""
					return m, nil
				case "ctrl+v":
					// Handle Ctrl+v paste explicitly to active input
					text, err := clipboard.ReadAll()
					if err == nil && text != "" {
						if m.focusedInput == 0 {
							m.input.SetValue(text)
						} else {
							m.nicknameInput.SetValue(text)
						}
					}
					return m, nil
				case "shift+tab", "tab", "ctrl+i", "down":
					// Toggle between address and nickname fields
					if m.focusedInput == 0 {
						val := strings.TrimSpace(m.input.Value())
						// Trigger ENS lookup if valid address
						if helpers.IsValidEthAddress(val) {
							newAddr := common.HexToAddress(val).Hex()
							// Trigger ENS lookup if connected and not already looking up this address
							if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
								m.ensLookupActive = true
								m.ensLookupAddr = newAddr
								m.focusedInput = 1
								m.input.Blur()
								m.nicknameInput.Focus()
								return m, lookupENS(m.ethClient, newAddr)
							}
						}
						m.focusedInput = 1
						m.input.Blur()
						m.nicknameInput.Focus()
					} else {
						m.focusedInput = 0
						m.nicknameInput.Blur()
						m.input.Focus()
					}
					return m, nil
				case "enter":
					// If on address field, check for .eth name or valid address
					if m.focusedInput == 0 {
						val := strings.TrimSpace(m.input.Value())
						
						// Check if it's a .eth ENS name
						if strings.HasSuffix(strings.ToLower(val), ".eth") {
							// Trigger forward ENS resolution
							if m.ethClient != nil {
								m.ensLookupActive = true
								m.ensLookupAddr = val // Store the ENS name being resolved
								return m, resolveENS(m.ethClient, val)
							}
							return m, nil
						}
						
						if helpers.IsValidEthAddress(val) {
							newAddr := common.HexToAddress(val).Hex()
							// Move to nickname field
							m.focusedInput = 1
							m.input.Blur()
							m.nicknameInput.Focus()
							// Trigger ENS reverse lookup if connected and not already looking up this address
							if m.ethClient != nil && (!m.ensLookupActive || m.ensLookupAddr != newAddr) {
								m.ensLookupActive = true
								m.ensLookupAddr = newAddr
								return m, lookupENS(m.ethClient, newAddr)
							}
							return m, nil
						}
						return m, nil
					}
					// If on nickname field, submit the form
					val := strings.TrimSpace(m.input.Value())
					if helpers.IsValidEthAddress(val) {
						newAddr := common.HexToAddress(val).Hex()

						// Check for duplicates
						for _, w := range m.accounts {
							if strings.EqualFold(w.Address, newAddr) {
								m.addError = "Duplicate address - wallet already exists"
								m.addErrTime = time.Now()
								m.input.SetValue("")
								m.nicknameInput.SetValue("")
								m.focusedInput = 0
								m.input.Focus()
								return m, nil
							}
						}

						// Create new wallet entry with nickname
						nickname := strings.TrimSpace(m.nicknameInput.Value())
						newWallet := config.WalletEntry{
							Address: newAddr,
							Name:    nickname,
							Active:  false,
						}
						m.accounts = append(m.accounts, newWallet)
						m.selectedWallet = len(m.accounts) - 1
						m.highlightedAddress = newAddr
						m.adding = false
						m.input.SetValue("")
						m.nicknameInput.SetValue("")
						m.input.Blur()
						m.nicknameInput.Blur()
						m.focusedInput = 0
						m.addError = ""
						// Save wallets to config
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						if nickname != "" {
							m.addLog("success", fmt.Sprintf("Added wallet `%s` with nickname `%s`", helpers.ShortenAddr(newAddr), nickname))
						} else {
							m.addLog("success", fmt.Sprintf("Added wallet `%s`", helpers.ShortenAddr(newAddr)))
						}
						// Load details for the newly added wallet if split view is enabled
						return m, m.loadSelectedWalletDetails()
					} else {
						m.addError = "Invalid Etherem Address"
						m.addErrTime = time.Now()
						m.input.SetValue("")
						m.nicknameInput.SetValue("")
						m.focusedInput = 0
						m.input.Focus()
						return m, nil
					}
					return m, nil
				}

				var cmd tea.Cmd
				if m.focusedInput == 0 {
					m.input, cmd = m.input.Update(msg)
				} else {
					m.nicknameInput, cmd = m.nicknameInput.Update(msg)
				}
				return m, cmd
			}

			// normal list controls
			switch msg.String() {
			case "up", "k":
				if m.selectedWallet > 0 {
					m.selectedWallet--
					if len(m.accounts) > 0 {
						m.highlightedAddress = m.accounts[m.selectedWallet].Address
					}
				}
				return m, nil

			case "down", "j":
				if m.selectedWallet < len(m.accounts)-1 {
					m.selectedWallet++
					if len(m.accounts) > 0 {
						m.highlightedAddress = m.accounts[m.selectedWallet].Address
					}
				}
				return m, nil

			case "a", "A":
				m.adding = true
				m.focusedInput = 0
				m.input.SetValue("")
				m.nicknameInput.SetValue("")
				m.input.Focus()
				m.nicknameInput.Blur()
				m.addError = ""
				m.ensLookupActive = false
				m.ensLookupAddr = ""
				return m, nil

			case " ":
				// Set selected wallet as active
				if len(m.accounts) > 0 {
					for i := range m.accounts {
						m.accounts[i].Active = (i == m.selectedWallet)
					}
					// Update active address to the newly activated wallet
					m.activeAddress = m.accounts[m.selectedWallet].Address
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
					m.addLog("info", fmt.Sprintf("Activated wallet `%s`", helpers.ShortenAddr(m.activeAddress)))

					// If split view is enabled, refresh details for the newly activated wallet
					if m.detailsInWallets {
						addr := m.accounts[m.selectedWallet].Address
						m.loading = true
						m.details = walletDetails{Address: addr}
						ethAddr := common.HexToAddress(addr)
						return m, loadDetails(m.ethClient, ethAddr, m.tokenWatch)
					}
				}
				return m, nil

		case "s", "S":
			m.activePage = pageSettings
			m.settingsMode = "list"
			return m, nil

		case "b", "B":
			m.activePage = pageDappBrowser
			m.dappMode = "list"
			return m, nil

		case "h", "H":

			case "esc":
				return m, tea.Quit

			case "delete", "backspace":
				// Show delete confirmation dialog
				if len(m.accounts) == 0 {
					return m, nil
				}
				m.showDeleteDialog = true
				m.deleteDialogYesSelected = true // Default to Yes button
				m.deleteDialogIdx = m.selectedWallet
				m.deleteDialogAddr = m.accounts[m.selectedWallet].Address
				return m, nil
			}
			return m, nil

		case pageDetails:
			// Don't handle keys if nicknaming form is active
			if !m.nicknaming {
				switch msg.String() {
				case "esc", "backspace":
					m.activePage = pageWallets
					// Load details for selected wallet if split view enabled
					return m, m.loadSelectedWalletDetails()

				case "r", "R":
					// refresh
					addr := common.HexToAddress(m.details.Address)
					m.loading = true
					m.addLog("info", fmt.Sprintf("Refreshing details for `%s`", helpers.ShortenAddr(m.details.Address)))
					return m, loadDetails(m.ethClient, addr, m.tokenWatch)

				case "n", "N":
					// nickname
					m.nicknaming = true
					m.createNicknameForm()
					return m, nil
				}
			}

		case pageDappBrowser:
			// Only handle list mode controls here (form handled at top of Update)
			if m.dappMode == "list" {
				switch msg.String() {
				case "esc":
					m.activePage = pageWallets
					// Load details for selected wallet if split view enabled
					return m, m.loadSelectedWalletDetails()

				case "enter":
					// Open Uniswap swap interface
					m.activePage = pageUniswap
					// Initialize Uniswap state with default values
					m.uniswapFromTokenIdx = 0 // Default to first token (ETH)
					m.uniswapToTokenIdx = 1   // Default to second token if available
					m.uniswapFromAmount = ""
					m.uniswapToAmount = ""
					m.uniswapFocusedField = 0
					m.uniswapShowingSelector = false
					m.uniswapSelectorFor = 0
					m.uniswapSelectorIdx = 0
					m.uniswapEstimating = false
					m.uniswapQuote = nil
					m.uniswapQuoteError = ""
					m.uniswapPriceImpactWarn = ""
					// Reset tracking state
					m.lastQuoteFromAmount = ""
					m.lastQuoteFromTokenIdx = -1
					m.lastQuoteToTokenIdx = -1
					return m, nil

				case "tab", "down", "right":
					// Cycle to next dApp (wraps around)
					if len(m.dapps) > 0 {
						m.selectedDappIdx = (m.selectedDappIdx + 1) % len(m.dapps)
					}
					return m, nil

				case "shift+tab", "up", "left":
					// Cycle to previous dApp (wraps around)
					if len(m.dapps) > 0 {
						m.selectedDappIdx--
						if m.selectedDappIdx < 0 {
							m.selectedDappIdx = len(m.dapps) - 1
						}
					}
					return m, nil

			case "a", "A":
				m.dappMode = "add"
				m.createAddDappForm()
				return m, nil

			case "e", "E":

			case "delete", "backspace":
				// Delete selected dApp
				if len(m.dapps) > 0 && m.selectedDappIdx < len(m.dapps) {
					deletedDapp := m.dapps[m.selectedDappIdx].Name
					m.dapps = append(m.dapps[:m.selectedDappIdx], m.dapps[m.selectedDappIdx+1:]...)
					if m.selectedDappIdx >= len(m.dapps) && m.selectedDappIdx > 0 {
						m.selectedDappIdx--
					}
					config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
					m.addLog("warning", fmt.Sprintf("Deleted dApp `%s`", deletedDapp))
				}
				return m, nil
				}
			}

		case pageSettings:
			if m.showRPCDeleteDialog {
				switch msg.String() {
				case "left", "right", "tab":
					m.deleteRPCDialogYesSelected = !m.deleteRPCDialogYesSelected
					return m, nil
				case "enter":
					if m.deleteRPCDialogYesSelected {
						idx := m.deleteRPCDialogIdx
						deletedName := m.deleteRPCDialogName
						if idx >= 0 && idx < len(m.rpcURLs) {
							m.rpcURLs = append(m.rpcURLs[:idx], m.rpcURLs[idx+1:]...)
							if m.selectedRPCIdx >= len(m.rpcURLs) && m.selectedRPCIdx > 0 {
								m.selectedRPCIdx--
							}
							config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
							m.addLog("warning", fmt.Sprintf("Deleted RPC endpoint `%s`", deletedName))
						}
						m.showRPCDeleteDialog = false
						return m, nil
					}
					m.showRPCDeleteDialog = false
					return m, nil
				case "esc":
					m.showRPCDeleteDialog = false
					return m, nil
				}
				return m, nil
			}
			// Only handle list mode controls here (form handled at top of Update)
			if m.settingsMode == "list" {
				switch msg.String() {
				case "esc":
					m.activePage = pageWallets
					// Load details for selected wallet if split view enabled
					return m, m.loadSelectedWalletDetails()

				case "a", "A":
					m.settingsMode = "add"
					m.createAddRPCForm()
					return m, nil

				case "e", "E":
					if len(m.rpcURLs) > 0 {
						m.settingsMode = "edit"
						m.createEditRPCForm(m.selectedRPCIdx)
					}
					return m, nil

				case "delete", "backspace":
					if len(m.rpcURLs) > 0 && m.selectedRPCIdx < len(m.rpcURLs) {
						m.showRPCDeleteDialog = true
						m.deleteRPCDialogYesSelected = true
						m.deleteRPCDialogIdx = m.selectedRPCIdx
						name := strings.TrimSpace(m.rpcURLs[m.selectedRPCIdx].Name)
						if name == "" {
							name = m.rpcURLs[m.selectedRPCIdx].URL
						}
						m.deleteRPCDialogName = name
					}
					return m, nil

				case "up", "k":
					if m.selectedRPCIdx > 0 {
						m.selectedRPCIdx--
					}
					return m, nil

				case "down", "j":
					if m.selectedRPCIdx < len(m.rpcURLs)-1 {
						m.selectedRPCIdx++
					}
					return m, nil

				case "enter", " ":
					// Set as active
					if len(m.rpcURLs) > 0 && m.selectedRPCIdx < len(m.rpcURLs) {
						for i := range m.rpcURLs {
							m.rpcURLs[i].Active = (i == m.selectedRPCIdx)
						}
						m.rpcURL = m.rpcURLs[m.selectedRPCIdx].URL
						config.Save(m.configPath, config.Config{RPCURLs: m.rpcURLs, Wallets: m.accounts, Dapps: m.dapps, Logger: m.logEnabled})
						// Set connecting state and reconnect with new RPC
						m.rpcConnecting = true
						m.rpcConnected = false
						return m, connectRPC(m.rpcURL)
					}
					return m, nil
				}
			}

		case pageUniswap:
			// Handle transaction result panel first
			if m.showTxResultPanel {
				switch msg.String() {
				case "esc", "enter":
					m.showTxResultPanel = false
					m.txResultHex = ""
					m.txResultEIP681 = ""
					m.txResultError = ""
					m.txResultPackaging = false
					return m, nil
				}
				return m, nil
			}

			// Handle token selector popup
			if m.uniswapShowingSelector {
				switch msg.String() {
				case "esc":
					m.uniswapShowingSelector = false
					return m, nil
				case "up", "k":
					if m.uniswapSelectorIdx > 0 {
						m.uniswapSelectorIdx--
					}
					return m, nil
				case "down", "j":
					// Build token list from wallet details
					tokens := m.buildTokenList()
					if m.uniswapSelectorIdx < len(tokens)-1 {
						m.uniswapSelectorIdx++
					}
					return m, nil
				case "enter":
					// Select token and close selector
					if m.uniswapSelectorFor == 0 {
						m.uniswapFromTokenIdx = m.uniswapSelectorIdx
					} else {
						m.uniswapToTokenIdx = m.uniswapSelectorIdx
					}
					m.uniswapShowingSelector = false
					// Trigger quote fetch since token selection changed
					return m, m.maybeRequestUniswapQuote()
				}
				return m, nil
			}

			// Main swap interface controls
			switch msg.String() {
			case "esc":
				m.activePage = pageDappBrowser
				return m, nil

			case "up", "k":
				// Navigate up through fields
				if m.uniswapFocusedField > 0 {
					// If leaving To field with value, trigger reverse quote
					if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
						m.uniswapFocusedField--
						m.uniswapEditingFrom = false
						m.uniswapEditingTo = false
						return m, m.maybeRequestReverseUniswapQuote()
					}
					m.uniswapFocusedField--
					// Reset editing flags when navigating to a field
					if m.uniswapFocusedField == 0 {
						m.uniswapEditingFrom = false
					} else if m.uniswapFocusedField == 1 {
						m.uniswapEditingTo = false
					}
				}
				return m, nil

			case "down", "j":
				// Navigate down through fields
				if m.uniswapFocusedField < 2 {
					// If leaving From field, trigger forward quote
					if m.uniswapFocusedField == 0 && m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
						m.uniswapFocusedField++
						if m.uniswapFocusedField == 1 {
							m.uniswapEditingTo = false
						}
						return m, m.maybeRequestUniswapQuote()
					}
					// If leaving To field, trigger reverse quote
					if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
						m.uniswapFocusedField++
						m.uniswapEditingTo = false
						return m, m.maybeRequestReverseUniswapQuote()
					}
					m.uniswapFocusedField++
					if m.uniswapFocusedField == 1 {
						m.uniswapEditingTo = false
					}
				}
				return m, nil

			case "tab":
				// Cycle through fields
				// If leaving From field, trigger forward quote
				if m.uniswapFocusedField == 0 && m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
					m.uniswapFocusedField = (m.uniswapFocusedField + 1) % 3
					// Reset editing flags when entering a field
					if m.uniswapFocusedField == 1 {
						m.uniswapEditingTo = false
					}
					return m, m.maybeRequestUniswapQuote()
				}
				// If leaving To field, trigger reverse quote
				if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
					m.uniswapFocusedField = (m.uniswapFocusedField + 1) % 3
					m.uniswapEditingTo = false
					return m, m.maybeRequestReverseUniswapQuote()
				}
				m.uniswapFocusedField = (m.uniswapFocusedField + 1) % 3
				// Reset editing flags when entering a field
				if m.uniswapFocusedField == 0 {
					m.uniswapEditingFrom = false
				} else if m.uniswapFocusedField == 1 {
					m.uniswapEditingTo = false
				}
				return m, nil

			case "shift+tab":
				// Cycle through fields in reverse
				// If leaving From field, trigger forward quote
				if m.uniswapFocusedField == 0 && m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
					m.uniswapFocusedField = (m.uniswapFocusedField - 1 + 3) % 3
					// Reset editing flags when entering a field
					if m.uniswapFocusedField == 1 {
						m.uniswapEditingTo = false
					}
					return m, m.maybeRequestUniswapQuote()
				}
				// If leaving To field, trigger reverse quote
				if m.uniswapFocusedField == 1 && m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
					m.uniswapFocusedField = (m.uniswapFocusedField - 1 + 3) % 3
					m.uniswapEditingFrom = false
					m.uniswapEditingTo = false
					return m, m.maybeRequestReverseUniswapQuote()
				}
				m.uniswapFocusedField = (m.uniswapFocusedField - 1 + 3) % 3
				// Reset editing flags when entering a field
				if m.uniswapFocusedField == 0 {
					m.uniswapEditingFrom = false
				} else if m.uniswapFocusedField == 1 {
					m.uniswapEditingTo = false
				}
				return m, nil

			case "enter":
				if m.uniswapFocusedField == 0 {
					// If user has been editing, move to next field instead of opening selector
					if m.uniswapEditingFrom {
						if m.uniswapFromAmount != "" {
							m.uniswapFocusedField++
							m.uniswapEditingTo = false
							return m, m.maybeRequestUniswapQuote()
						}
						m.uniswapFocusedField++
						m.uniswapEditingTo = false
						return m, nil
					}
					// Otherwise, open token selector for "from" field
					var cmd tea.Cmd
					if m.uniswapFromAmount != "" {
						cmd = m.maybeRequestUniswapQuote()
					}
					m.uniswapShowingSelector = true
					m.uniswapSelectorFor = 0
					m.uniswapSelectorIdx = m.uniswapFromTokenIdx
					return m, cmd
				} else if m.uniswapFocusedField == 1 {
					// If user has been editing To field, move to next field and trigger reverse quote
					if m.uniswapEditingTo {
						if m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
							m.uniswapFocusedField++
							m.uniswapEditingTo = false
							return m, m.maybeRequestReverseUniswapQuote()
						}
						m.uniswapFocusedField++
						m.uniswapEditingTo = false
						return m, nil
					}
					// Otherwise, open token selector for "to" field
					m.uniswapShowingSelector = true
					m.uniswapSelectorFor = 1
					m.uniswapSelectorIdx = m.uniswapToTokenIdx
					return m, nil
				} else if m.uniswapFocusedField == 2 {
					// Execute swap - package transaction and show QR code
					if m.uniswapFromAmount == "" || m.uniswapToAmount == "" {
						m.addLog("error", "Please enter an amount and get a quote first")
						return m, nil
					}
					if m.uniswapQuote == nil {
						m.addLog("error", "Please get a swap quote first")
						return m, nil
					}

					tokens := m.buildTokenList()
					if m.uniswapFromTokenIdx < 0 || m.uniswapFromTokenIdx >= len(tokens) {
						return m, nil
					}
					if m.uniswapToTokenIdx < 0 || m.uniswapToTokenIdx >= len(tokens) {
						return m, nil
					}

					fromToken := tokens[m.uniswapFromTokenIdx]
					toToken := tokens[m.uniswapToTokenIdx]

					m.addLog("info", fmt.Sprintf("Packaging swap: %s %s → %s %s", m.uniswapFromAmount, fromToken.Symbol, m.uniswapToAmount, toToken.Symbol))
					m.showTxResultPanel = true
					m.txResultPackaging = true
					m.txResultHex = ""
					m.txResultError = ""
					return m, packageSwapTransaction(m.activeAddress, fromToken, toToken, m.uniswapFromAmount, m.uniswapQuote.AmountOut, m.rpcURL)
				}
				return m, nil

			case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", ".":
				// Allow numeric input for amount when focused on from field
				if m.uniswapFocusedField == 0 {
					char := msg.String()
					// If not currently editing and field has a non-zero value, clear it first
					if !m.uniswapEditingFrom && m.uniswapFromAmount != "" && m.uniswapFromAmount != "0" {
						m.uniswapFromAmount = ""
					}
					// Prevent multiple decimal points
					if char == "." && strings.Contains(m.uniswapFromAmount, ".") {
						return m, nil
					}
					m.uniswapFromAmount += char
					m.uniswapEditingFrom = true // Mark that user is actively editing
					// Quote will be fetched when user leaves the field
					return m, nil
				} else if m.uniswapFocusedField == 1 {
					char := msg.String()
					// If not currently editing and field has a non-zero value, clear it first
					if !m.uniswapEditingTo && m.uniswapToAmount != "" && m.uniswapToAmount != "0" {
						m.uniswapToAmount = ""
					}
					// Prevent multiple decimal points
					if char == "." && strings.Contains(m.uniswapToAmount, ".") {
						return m, nil
					}
					m.uniswapToAmount += char
					m.uniswapEditingTo = true // Mark that user is actively editing
					return m, nil
				}
				return m, nil

			case "backspace":
				// Delete last character from amount
				if m.uniswapFocusedField == 0 && len(m.uniswapFromAmount) > 0 {
					m.uniswapFromAmount = m.uniswapFromAmount[:len(m.uniswapFromAmount)-1]
					m.uniswapEditingFrom = true // Mark that user is actively editing
					// Quote will be fetched when user leaves the field
					return m, nil
				} else if m.uniswapFocusedField == 1 && len(m.uniswapToAmount) > 0 {
					m.uniswapToAmount = m.uniswapToAmount[:len(m.uniswapToAmount)-1]
					m.uniswapEditingTo = true // Mark that user is actively editing
					return m, nil
				}
				return m, nil

			case "m", "M":
				// Max: populate From field with full balance
				if m.uniswapFocusedField == 0 {
					tokens := m.buildTokenList()
					if m.uniswapFromTokenIdx >= 0 && m.uniswapFromTokenIdx < len(tokens) {
						fromToken := tokens[m.uniswapFromTokenIdx]
						if fromToken.Balance != nil && fromToken.Balance.Sign() > 0 {
							// Convert balance to decimal string
							divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
							balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(fromToken.Balance), divisor)
							m.uniswapFromAmount = balanceFloat.Text('f', 6)
							m.uniswapEditingFrom = true // Mark that user is actively editing
							// Trigger quote fetch immediately for max
							m.addLog("info", fmt.Sprintf("Max balance: %s %s", m.uniswapFromAmount, fromToken.Symbol))
							return m, m.maybeRequestUniswapQuote()
						}
					}
				}
				return m, nil
			}
		}

	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft {
			// Check if click is on any registered clickable address
			for _, area := range m.clickableAreas {
				if msg.X >= area.X && msg.X < area.X+area.Width &&
					msg.Y >= area.Y && msg.Y < area.Y+area.Height {
					// If on details page and clicking same address, copy to clipboard
					if m.activePage == pageDetails && area.Address == m.details.Address {
						return m, copyToClipboard(area.Address)
					}
					// Otherwise navigate to wallet details
					// Find wallet index
					for i, w := range m.accounts {
						if strings.EqualFold(w.Address, area.Address) {
							m.selectedWallet = i
							break
						}
					}
					m.highlightedAddress = area.Address
					m.activePage = pageDetails
					m.loading = true
					m.details = walletDetails{Address: area.Address}
					ethAddr := common.HexToAddress(area.Address)
					return m, loadDetails(m.ethClient, ethAddr, m.tokenWatch)
				}
			}

			// Handle click on transaction JSON in tx result panel
			// Make the entire panel clickable (simpler than precise coordinate tracking)
			if m.showTxResultPanel && m.txResultHex != "" {
				m.addLog("info", "Copied transaction JSON to clipboard")
				return m, copyTxJsonToClipboard(m.txResultHex)
			}

			// Legacy: handle address click on details page if no area matched
			if m.activePage == pageDetails && m.details.Address != "" {
				if msg.Y == m.addressLineY {
					return m, copyToClipboard(m.details.Address)
				}
			}
		}

	case clipboardCopiedMsg:
		m.copiedMsg = "✓ Copied address to clipboard"
		m.copiedMsgTime = time.Now()
		return m, clearClipboardMsg()

	case txJsonCopiedMsg:
		m.txCopiedMsg = "✓ Copied to clipboard"
		m.txCopiedMsgTime = time.Now()
		return m, clearClipboardMsg()

	case uniswapQuoteMsg:
		m.uniswapEstimating = false
		if msg.err != nil {
			m.uniswapQuoteError = msg.err.Error()
			m.uniswapQuote = nil
			m.uniswapToAmount = ""
			m.uniswapFromAmount = ""
			m.uniswapPriceImpactWarn = ""
			m.addLog("error", fmt.Sprintf("Swap quote error: %v", msg.err))
			return m, nil
		}

		m.uniswapQuoteError = ""
		m.uniswapQuote = msg.quote
		m.uniswapPriceImpactWarn = ""

		if msg.quote != nil {
			// Log detailed quote information
			tokens := m.buildTokenList()
			fromToken := tokens[m.uniswapFromTokenIdx]
			toToken := tokens[m.uniswapToTokenIdx]

			// Check if this is a reverse quote (To amount was entered, calculate From)
			isReverseQuote := m.uniswapFromAmount == "" && m.uniswapToAmount != ""

			if isReverseQuote {
				// Calculate required input amount with proper decimals
				divisorIn := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fromToken.Decimals)), nil))
				amountInFormatted := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.AmountIn), divisorIn)
				m.uniswapFromAmount = amountInFormatted.Text('f', 6)
				m.uniswapEditingFrom = false

				m.addLog("info", fmt.Sprintf("📊 Reverse Quote: %s → %s", fromToken.Symbol, toToken.Symbol))
				m.addLog("info", fmt.Sprintf("  Amount In: %s %s", m.uniswapFromAmount, fromToken.Symbol))
				m.addLog("info", fmt.Sprintf("  Amount Out: %s %s", m.uniswapToAmount, toToken.Symbol))
			} else {
				// Normal forward quote
				m.addLog("info", fmt.Sprintf("📊 Swap Quote: %s → %s", fromToken.Symbol, toToken.Symbol))
				m.addLog("info", fmt.Sprintf("  Amount In: %s %s", m.uniswapFromAmount, fromToken.Symbol))
				
				// Calculate output amount with proper decimals
				divisorOut := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(toToken.Decimals)), nil))
				amountOutFormatted := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.AmountOut), divisorOut)
				m.uniswapToAmount = amountOutFormatted.Text('f', 6)

				m.addLog("info", fmt.Sprintf("  Amount Out: %s %s", m.uniswapToAmount, toToken.Symbol))
			}

			m.addLog("info", fmt.Sprintf("  Price Impact: %.4f%%", msg.quote.PriceImpact))
			
			// Log reserves
			divisor0 := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
			reserve0Fmt := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.Token0Reserve), divisor0)
			reserve1Fmt := new(big.Float).Quo(new(big.Float).SetInt(msg.quote.Token1Reserve), divisor0)
			m.addLog("info", fmt.Sprintf("  Reserves: %s / %s", reserve0Fmt.Text('f', 2), reserve1Fmt.Text('f', 2)))

			// Check for high price impact
			if msg.quote.PriceImpact > 1.0 {
				m.uniswapPriceImpactWarn = fmt.Sprintf("⚠ High price impact: %.2f%%", msg.quote.PriceImpact)
				m.addLog("warn", m.uniswapPriceImpactWarn)
			} else if msg.quote.PriceImpact > 0.5 {
				m.uniswapPriceImpactWarn = fmt.Sprintf("⚠ Moderate price impact: %.2f%%", msg.quote.PriceImpact)
			}
		}
		return m, nil

	case ensLookupResultMsg:
		m.ensLookupActive = false
		// Always log debug info
		if msg.debugInfo != "" {
			m.addLog("info", fmt.Sprintf("ENS debug: %s", msg.debugInfo))
		}
		if msg.err == nil && msg.ensName != "" && msg.address == m.ensLookupAddr {
			// Auto-populate nickname field if it's empty
			if strings.TrimSpace(m.nicknameInput.Value()) == "" {
				m.nicknameInput.SetValue(msg.ensName)
			}
			m.addLog("success", fmt.Sprintf("Found ENS name: %s", msg.ensName))
		} else if msg.err != nil && msg.address == m.ensLookupAddr {
			m.addLog("error", fmt.Sprintf("ENS lookup error: %v", msg.err))
		} else if msg.address == m.ensLookupAddr {
			m.addLog("info", "No ENS name found for address: " + helpers.FadeString(helpers.ShortenAddr(msg.address), "#F25D94", "#EDFF82"))
		}
		return m, nil

	case ensForwardResolveMsg:
		m.ensLookupActive = false
		// Always log debug info
		if msg.debugInfo != "" {
			m.addLog("info", fmt.Sprintf("ENS resolve debug: %s", msg.debugInfo))
		}
		if msg.err == nil && msg.address != "" {
			// Successfully resolved - populate address field with resolved address
			m.input.SetValue(msg.address)
			// Populate nickname field with the ENS name
			if strings.TrimSpace(m.nicknameInput.Value()) == "" {
				m.nicknameInput.SetValue(msg.ensName)
			}
			// Move to nickname field for confirmation
			m.focusedInput = 1
			m.input.Blur()
			m.nicknameInput.Focus()
			m.addLog("success", fmt.Sprintf("Resolved %s to %s", msg.ensName, helpers.ShortenAddr(msg.address)))
		} else if msg.err != nil {
			m.addLog("error", fmt.Sprintf("ENS resolution error: %v", msg.err))
			m.addError = fmt.Sprintf("Failed to resolve %s", msg.ensName)
			m.addErrTime = time.Now()
		}
		return m, nil

	default:
		// Clear clipboard message after timeout
		if msg, ok := msg.(struct{ clearClipboard bool }); ok && msg.clearClipboard {
			if time.Since(m.copiedMsgTime) >= 2*time.Second {
				m.copiedMsg = ""
			}
			if time.Since(m.txCopiedMsgTime) >= 2*time.Second {
				m.txCopiedMsg = ""
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// -------------------- VIEW --------------------

func (m model) renderDeleteDialog() string {
	var (
		dialogBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#874BFD")).
				Padding(1, 0).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(true)

		buttonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#888B7E")).
				Padding(0, 3).
				MarginTop(1)

		activeButtonStyle = buttonStyle.Copy().
					Foreground(lipgloss.Color("#FFF7DB")).
					Background(lipgloss.Color("#F25D94")).
					MarginRight(2).
					Underline(true)
	)
	msg := helpers.FadeString("Are you sure you want to delete the account "+helpers.ShortenAddr(m.deleteDialogAddr)+"?", "#F25D94", "#EDFF82")
	question := lipgloss.NewStyle().Width(50).Align(lipgloss.Center).Render(msg)

	// Apply active style to the selected button
	var okButton, cancelButton string
	if m.deleteDialogYesSelected {
		okButton = activeButtonStyle.Render("Yes")
		cancelButton = buttonStyle.Render("No")
	} else {
		okButton = buttonStyle.Copy().MarginRight(2).Render("Yes")
		cancelButton = activeButtonStyle.Copy().MarginRight(0).Render("No")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, okButton, cancelButton)
	ui := lipgloss.JoinVertical(lipgloss.Center, question, buttons)

	dialog := dialogBoxStyle.Render(ui)

	// Center the dialog on screen
	return lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m model) renderRPCDeleteDialog() string {
	var (
		dialogBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#874BFD")).
				Padding(1, 0).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(true)

		buttonStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#888B7E")).
				Padding(0, 3).
				MarginTop(1)

		activeButtonStyle = buttonStyle.Copy().
				Foreground(lipgloss.Color("#FFF7DB")).
				Background(lipgloss.Color("#F25D94")).
				MarginRight(2).
				Underline(true)
	)
	msg := helpers.FadeString("Are you sure you want to delete the RPC endpoint "+m.deleteRPCDialogName+"?", "#F25D94", "#EDFF82")
	question := lipgloss.NewStyle().Width(50).Align(lipgloss.Center).Render(msg)

	var okButton, cancelButton string
	if m.deleteRPCDialogYesSelected {
		okButton = activeButtonStyle.Render("Yes")
		cancelButton = buttonStyle.Render("No")
	} else {
		okButton = buttonStyle.Copy().MarginRight(2).Render("Yes")
		cancelButton = activeButtonStyle.Copy().MarginRight(0).Render("No")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, okButton, cancelButton)
	ui := lipgloss.JoinVertical(lipgloss.Center, question, buttons)

	dialog := dialogBoxStyle.Render(ui)

	return lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (m *model) renderTxResultContent() string {
	txResultContent := styles.TitleStyle.Render("Transaction Ready To Sign (EIP-4527)") + "\n\n"
	
	if m.txResultPackaging {
		txResultContent += m.spin.View() + " Packaging transaction..."
	} else if m.txResultError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
		txResultContent += errorStyle.Render("Error: " + m.txResultError)
		txResultContent += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("Press ESC or Enter to close")
	} else {
		qrCode := rpc.GenerateQRCode(m.txResultEIP681)
		qrStyle := lipgloss.NewStyle()
		txResultContent += qrStyle.Render(qrCode) + "\n"

		txResultContent += lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("EIP-4527 Transaction JSON:") + "\n\n"
		txResultContent += m.txResultHex

		txResultContent += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("Scan the QR code with your wallet app to sign this transaction")
		txResultContent += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("Click anywhere or press Ctrl+C to copy • Press ESC or Enter to close")
		
		// Show copied message if present
		if m.txCopiedMsg != "" {
			txResultContent += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Render(m.txCopiedMsg)
		}
	}
	return txResultContent
}

func (m *model) renderTxResultPanel() string {
	contentWidth := max(0, m.w-8)
	centeredContent := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(m.renderTxResultContent())
	content := panelStyle.Width(max(0, m.w-4)).Render(centeredContent)
	return appStyle.Render(lipgloss.Place(
		m.w, m.h,
		lipgloss.Center, lipgloss.Center,
		content,
	))
}

func (m model) globalHeader() string {
	availableWidth := max(0, m.w-8) // Account for panel padding
	
	// Active Address (the one marked with ★)
	var addrDisplay string
	if m.activeAddress != "" {
		addrDisplay = lipgloss.NewStyle().
			Foreground(cAccent2).
			Bold(true).
			Render("Active Address: " + helpers.FadeString(helpers.ShortenAddr(m.activeAddress), "#F25D94", "#EDFF82"))
	} else {
		addrDisplay = lipgloss.NewStyle().
			Foreground(cMuted).
			Render("Active Address: No selection")
	}

	// RPC Status with green dot
	var statusIcon string
	var statusColor lipgloss.Color
	var statusText string

	if m.rpcURL == "" {
		statusIcon = "○"
		statusColor = lipgloss.Color("#c01c28")
		statusText = "No RPC"
	} else if m.rpcConnecting {
		statusIcon = "○"
		statusColor = lipgloss.Color("#c01c28")
		statusText = "Connecting..."
	} else if !m.rpcConnected {
		statusIcon = "○"
		statusColor = lipgloss.Color("#c01c28")
		statusText = "Connection Failed"
	} else {
		statusIcon = "●"
		statusColor = cAccent
		// Find active RPC name
		for _, r := range m.rpcURLs {
			if r.Active && r.URL == m.rpcURL {
				statusText = r.Name
				break
			}
		}
		if statusText == "" {
			statusText = "Connected"
		}
	}

	rpcDisplay := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true).
		Render(statusIcon + " " + statusText)

	// Center title
	titleStyle := lipgloss.NewStyle().
		Foreground(cAccent).
		Bold(true)
	titleText := titleStyle.Render(helpers.FadeString("domestic system", "#7EE787", "#82CFFD"))
	
	// Calculate widths
	addrWidth := lipgloss.Width(addrDisplay)
	rpcWidth := lipgloss.Width(rpcDisplay)
	titleWidth := lipgloss.Width(titleText)
	
	// Calculate spacing to center the title
	totalOtherWidth := addrWidth + rpcWidth + titleWidth
	
	var headerLine string
	if totalOtherWidth+4 > availableWidth {
		// Not enough space, stack vertically
		headerLine = addrDisplay + "\n" + titleText + "\n" + rpcDisplay
	} else {
		// Three-column layout: Address | Title (centered) | RPC
		// Calculate how much space for padding
		remainingSpace := availableWidth - totalOtherWidth
		leftPadding := remainingSpace / 2
		rightPadding := remainingSpace - leftPadding
		
		leftSpacer := strings.Repeat(" ", max(1, leftPadding))
		rightSpacer := strings.Repeat(" ", max(1, rightPadding))
		
		headerLine = addrDisplay + leftSpacer + titleText + rightSpacer + rpcDisplay
	}

	// Add separator line
	separator := lipgloss.NewStyle().
		Foreground(cBorder).
		Render(strings.Repeat("─", availableWidth))

	return headerLine + "\n" + separator
}

func (m model) View() string {
	// Clear clickable areas for fresh render
	m.clickableAreas = nil

	// Render global header outside of page content
	globalHdr := m.globalHeader()
	headerPanel := panelStyle.Width(max(0, m.w-2)).Render(globalHdr)

	// Register global header address as clickable (approximate position)
	if m.activeAddress != "" {
		// Header address is at approximately (4, 1) accounting for panel padding
		m.clickableAreas = append(m.clickableAreas, clickableArea{
			X:       4,
			Y:       1,
			Width:   42, // Ethereum address length
			Height:  1,
			Address: m.activeAddress,
		})
	}

	var pageContent string
	var nav string

	switch m.activePage {
	case pageHome:
		// TODO: home view not implemented yet
		// if m.homeForm == nil {
		// 	m.homeForm = home.CreateForm()
		// }
		// homeContent := home.Render(m.homeForm)
		pageContent = panelStyle.Width(max(0, m.w-2)).Render("Home view not implemented")
		nav = "" // home.Nav(m.w - 2)

	case pageWallets:
		walletsContent, _ := wallets.Render(m.accounts, m.selectedWallet, m.addError)

		// Show add wallet form if in adding mode
		if m.adding {
			inputView := m.input.View() + "\n" + m.nicknameInput.View() + "\n"

			// Show ENS lookup status
			if m.ensLookupActive {
				inputView += m.spin.View() + " ENS lookup…\n"
			}

			inputView += hotkeyStyle.Render("Tab") + " next field   " +
				hotkeyStyle.Render("Enter") + " next/save   " +
				hotkeyStyle.Render("Esc") + " cancel   " +
				hotkeyStyle.Render("Ctrl+v") + " paste"

			// Show error message if present and recent
			if m.addError != "" && time.Since(m.addErrTime) < 3*time.Second {
				errorStyle := lipgloss.NewStyle().Foreground(cWarn).Bold(true)
				inputView += "\n" + errorStyle.Render(m.addError)
			}

			addBoxView := "\n\n" + panelStyle.
				BorderForeground(cAccent2).
				Render(inputView)
			walletsContent += addBoxView
		}

		// If detailsInWallets is enabled and we have a selected wallet, show split view
		if m.detailsInWallets && len(m.accounts) > 0 {
			// Convert local walletDetails to rpc.WalletDetails
			rpcDetails := rpc.WalletDetails{
				Address:    m.details.Address,
				EthWei:     m.details.EthWei,
				LoadedAt:   m.details.LoadedAt,
				ErrMessage: m.details.ErrMessage,
			}
			for _, t := range m.details.Tokens {
				rpcDetails.Tokens = append(rpcDetails.Tokens, rpc.TokenBalance{
					Symbol:   t.Symbol,
					Decimals: t.Decimals,
					Balance:  t.Balance,
				})
			}

			detailsContent := details.Render(rpcDetails, m.accounts, m.loading, m.copiedMsg, m.spin.View())

			// Show send form if active
			// Show transaction result panel if active
			if m.showTxResultPanel {
				detailsContent = m.renderTxResultContent()
				// Show send form if active
			} else if m.showSendForm && m.sendForm != nil {
				sendFormContent := styles.TitleStyle.Render("Send Transaction") + "\n\n" + m.sendForm.View()
				detailsContent = sendFormContent
			} else if m.details.EthWei != nil && m.details.EthWei.Cmp(big.NewInt(0)) > 0 {
				// Add send button if ETH balance > 0 and form is not active
				var sendButtonStyle lipgloss.Style
				if m.sendButtonFocused {
					sendButtonStyle = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#FFF7DB")).
						Background(lipgloss.Color("#F25D94")).
						Padding(0, 3).
						MarginTop(2).
						Underline(true)
				} else {
					sendButtonStyle = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#FFF7DB")).
						Background(lipgloss.Color("#888B7E")).
						Padding(0, 3).
						MarginTop(2)
				}
				sendButton := sendButtonStyle.Render("Send")
				detailsContent += "\n\n" + sendButton

				// Add hint text
				if !m.sendButtonFocused {
					hintText := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#666666")).
						MarginTop(1).
						Render("Press Tab to select")
					detailsContent += "\n" + hintText
				} else {
					hintText := lipgloss.NewStyle().
						Foreground(lipgloss.Color("#666666")).
						MarginTop(1).
						Render("Press Enter to send")
					detailsContent += "\n" + hintText
				}
			}

			// Calculate panel widths (split 40/60)
			listWidth := max(0, (m.w*4)/10-2)
			detailsWidth := max(0, (m.w*6)/10-2)

			// Get the height of the left panel content to match it on the right
			leftPanel := panelStyle.Width(listWidth).Render(walletsContent)
			leftPanelHeight := lipgloss.Height(leftPanel)

			// Set the right panel to match the left panel height
			rightPanel := panelStyle.
				Width(detailsWidth + 1).
				Height(leftPanelHeight - 2).
				Render(detailsContent)

			pageContent = lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
		} else {
			pageContent = panelStyle.Width(max(0, m.w-2)).Render(walletsContent)
		}
		nav = wallets.Nav(m.w - 2)

		// Render delete confirmation dialog overlay
		if m.showDeleteDialog {
			// Dialog overlays the current view
			return m.renderDeleteDialog()
		}

		if m.showTxResultPanel {
			return m.renderTxResultPanel()
		}

	case pageDappBrowser:
		dappBrowserContent := dapps.Render(m.dapps, m.selectedDappIdx)

		// Show form if in add/edit mode
		if (m.dappMode == "add" || m.dappMode == "edit") && m.form != nil {
			dappBrowserContent = styles.TitleStyle.Render("dApp Browser") + "\n\n" + m.form.View()
		}

		pageContent = panelStyle.Width(max(0, m.w-2)).Render(dappBrowserContent)
		nav = dapps.Nav(m.w-2, m.dappMode)

	case pageDetails:
		// Convert local walletDetails to rpc.WalletDetails
		rpcDetails := rpc.WalletDetails{
			Address:    m.details.Address,
			EthWei:     m.details.EthWei,
			LoadedAt:   m.details.LoadedAt,
			ErrMessage: m.details.ErrMessage,
		}
		for _, t := range m.details.Tokens {
			rpcDetails.Tokens = append(rpcDetails.Tokens, rpc.TokenBalance{
				Symbol:   t.Symbol,
				Decimals: t.Decimals,
				Balance:  t.Balance,
			})
		}

		detailsContent := details.Render(rpcDetails, m.accounts, m.loading, m.copiedMsg, m.spin.View())
		pageContent = panelStyle.Width(max(0, m.w-2)).Render(detailsContent)
		nav = details.Nav(m.w-2, m.nicknaming)

	case pageSettings:
		settingsContent := settings.Render(m.rpcURLs, m.selectedRPCIdx)

		// Show form if in add/edit mode
		if (m.settingsMode == "add" || m.settingsMode == "edit") && m.form != nil {
			settingsContent = styles.TitleStyle.Render("RPC Settings") + "\n\n" + m.form.View()
		}

		pageContent = panelStyle.Width(max(0, m.w-2)).Render(settingsContent)
		nav = settings.Nav(m.w-2, m.settingsMode)

		if m.showRPCDeleteDialog {
			return m.renderRPCDeleteDialog()
		}

	case pageUniswap:
		// Build token list from wallet details
		tokens := m.buildTokenList()
		
		// If showing token selector, render popup
		if m.uniswapShowingSelector {
			uniswapView := uniswap.RenderTokenSelector(
				m.w,
				m.h-8, // Account for header and nav
				tokens,
				m.uniswapSelectorIdx,
				m.uniswapSelectorFor == 0,
			)
			pageContent = uniswapView
			nav = uniswap.Nav(m.w - 2)
		} else {
			// Render main swap interface
			uniswapView := uniswap.Render(
				m.w-2,
				m.h-8, // Account for header and nav
				tokens,
				m.uniswapFromTokenIdx,
				m.uniswapToTokenIdx,
				m.uniswapFromAmount,
				m.uniswapToAmount,
				m.uniswapFocusedField,
				m.uniswapEstimating,
				m.uniswapPriceImpactWarn,
			)
			// Wrap in panel style to constrain properly
			pageContent = panelStyle.Width(max(0, m.w-2)).Render(uniswapView)
			nav = uniswap.Nav(m.w - 2)
		}

		// Show transaction result panel overlay if active
		if m.showTxResultPanel {
			return m.renderTxResultPanel()
		}
	}

	// Render log panel only if enabled
	var logPanel string
	if m.logEnabled {
		logPanel = m.renderLogPanel()
		content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav, logPanel)
		return appStyle.Render(content)
	}

	// Use lipgloss to join sections vertically (without log panel)
	content := lipgloss.JoinVertical(lipgloss.Left, headerPanel, pageContent, nav)
	return appStyle.Render(content)
}

func (m model) renderLogPanel() string {
	title := lipgloss.NewStyle().
		Foreground(cAccent2).
		Bold(true).
		Render("Log")

	// Calculate available height for log panel
	// Account for: header (3 lines), nav (1 line), title + borders (4 lines), margins (2 lines)
	reservedHeight := 10
	availableHeight := max(5, m.h-reservedHeight)

	// Limit max height to 1/3 of screen or 15 lines, whichever is smaller
	maxLogHeight := min(m.h/3, 15)
	logPanelHeight := min(availableHeight, maxLogHeight)

	// Update viewport height dynamically
	m.logViewport.Height = logPanelHeight

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cBorder).
		Padding(0, 1).
		Width(max(0, m.w-2)).
		Height(logPanelHeight + 2) // +2 for title and spacing

	if !m.logReady {
		initMsg := "initializing...\n" + m.logSpinner.View()
		return border.Render(title + "\n\n" + initMsg)
	}

	// Show scrollbar info if content is larger than viewport
	scrollInfo := ""
	if m.logViewport.TotalLineCount() > 0 {
		scrollPercent := int(m.logViewport.ScrollPercent() * 100)
		if m.logViewport.TotalLineCount() > m.logViewport.Height {
			scrollInfo = lipgloss.NewStyle().
				Foreground(cMuted).
				Render(fmt.Sprintf(" [%d%%]", scrollPercent))
		}
	}

	titleWithScroll := title + scrollInfo

	return border.Render(titleWithScroll + "\n\n" + m.logViewport.View())
}

func key(s string) string {
	return hotkeyKeyStyle.Render(s)
}

func rpcStatus(url string, c *rpc.Client) string {
	if url == "" {
		return "not set"
	}
	if c == nil {
		return "connecting/failed"
	}
	return "connected"
}

func rainbow(base lipgloss.Style, s string, colors []color.Color) string {
	var str string
	for i, ss := range s {
		color, _ := colorful.MakeColor(colors[i%len(colors)])
		str = str + base.Foreground(lipgloss.Color(color.Hex())).Render(string(ss))
	}
	return str
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// -------------------- MAIN --------------------

func main() {
	m := newModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
