package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm-wallet-tui/config"
	"charm-wallet-tui/helpers"
	"charm-wallet-tui/rpc"
	"charm-wallet-tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/ethereum/go-ethereum/common"
)

// -------------------- MODEL --------------------

// model represents the application state following The Elm Architecture
type model struct {
	w, h int

	activePage config.Page

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
	details       config.WalletDetails
	detailsCache  map[string]config.WalletDetails // cache wallet details by address
	rpcURL        string
	ethClient     *rpc.Client
	rpcConnected  bool // true if RPC is successfully connected
	rpcConnecting bool // true if connection attempt is in progress

	// token watchlist (simple starter set)
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
	clickableAreas []config.ClickableArea

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
	txResultFormat    string
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

	// Double-click detection for header address
	lastClickTime time.Time
	lastClickX    int
	lastClickY    int

	// Account list popup (shown on double-click of active address in header)
	showAccountListPopup   bool
	accountListSelectedIdx int
	headerAddrX            int // X position of active address in header
	headerAddrY            int // Y position of active address in header
	headerAddrWidth        int // Width of active address display in header
}

// walletItem is a list item for the bubble-tea list component
// It has methods so it must stay in main package
type walletItem struct {
	addr string
}

func (w walletItem) Title() string       { return helpers.ShortenAddr(w.addr) }
func (w walletItem) Description() string { return w.addr }
func (w walletItem) FilterValue() string { return w.addr }

// -------------------- INIT --------------------

// newModel creates and initializes a new model with configuration from disk
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
	in.PromptStyle = lipgloss.NewStyle().Foreground(styles.CAccent)
	in.TextStyle = lipgloss.NewStyle().Foreground(styles.CText)
	in.Cursor.Style = lipgloss.NewStyle().Foreground(styles.CAccent2)
	in.CharLimit = 42
	in.Width = 48

	// input for nickname
	nicknameIn := textinput.New()
	nicknameIn.Placeholder = "Optional nickname"
	nicknameIn.Prompt = "Nickname: "
	nicknameIn.PromptStyle = lipgloss.NewStyle().Foreground(styles.CAccent)
	nicknameIn.TextStyle = lipgloss.NewStyle().Foreground(styles.CText)
	nicknameIn.Cursor.Style = lipgloss.NewStyle().Foreground(styles.CAccent2)
	nicknameIn.CharLimit = 50
	nicknameIn.Width = 48

	// spinner
	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = lipgloss.NewStyle().Foreground(styles.CAccent2)

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
		Foreground(styles.CText).
		Background(styles.CPanel)

	// Initialize log spinner
	logSpin := spinner.New()
	logSpin.Spinner = spinner.Dot
	logSpin.Style = lipgloss.NewStyle().Foreground(styles.CAccent2)

	m := model{
		activePage:         config.PageWallets,
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
		detailsCache:       make(map[string]config.WalletDetails),
		dapps:              cfg.Dapps,
		dappMode:           "list",
		selectedDappIdx:    0,
		detailsInWallets:   true, // Enable split panel view by default
	}

	return m
}

// Init implements tea.Model interface and returns initial commands
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
