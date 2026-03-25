package store

import (
	"context"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const erc20LookupABI = `[
  {"name":"name",    "type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"string"}]},
  {"name":"symbol",  "type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"string"}]},
  {"name":"decimals","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint8"}]}
]`

// HasERC20Token reports whether address is already in the erc20_tokens table.
func (s *Store) HasERC20Token(address common.Address) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM erc20_tokens WHERE address = ?`,
		address.Hex(),
	).Scan(&n)
	return n > 0, err
}

// SaveERC20Token upserts a token record. Silently ignores duplicates.
func (s *Store) SaveERC20Token(address common.Address, name, symbol string, decimals uint8) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO erc20_tokens (address, name, symbol, decimals)
		VALUES (?, ?, ?, ?)`,
		address.Hex(), name, symbol, int(decimals),
	)
	return err
}

// EnsureERC20Token looks up and stores the name, symbol, and decimals for
// address if it is not already in the table. It dials httpURL for the
// eth_call, so httpURL must use the http:// or https:// scheme.
// The zero address (native ETH) is recorded as name="Ether", symbol="ETH".
// Tokens that do not implement name()/symbol() get empty strings.
// Returns without error if the token is already cached.
func (s *Store) EnsureERC20Token(ctx context.Context, httpURL string, address common.Address) error {
	cached, err := s.HasERC20Token(address)
	if err != nil || cached {
		return err
	}
	client, err := ethclient.DialContext(ctx, httpURL)
	if err != nil {
		return err
	}
	defer client.Close()
	return s.ensureERC20TokenWithClient(ctx, client, address)
}

// EnsureERC20TokenWithClient is like EnsureERC20Token but reuses an existing
// ethclient connection. Use this from paths that have already dialled a client
// (e.g. IndexV4Backfill) to avoid opening extra connections.
func (s *Store) EnsureERC20TokenWithClient(ctx context.Context, client *ethclient.Client, address common.Address) error {
	cached, err := s.HasERC20Token(address)
	if err != nil || cached {
		return err
	}
	return s.ensureERC20TokenWithClient(ctx, client, address)
}

func (s *Store) ensureERC20TokenWithClient(ctx context.Context, client *ethclient.Client, address common.Address) error {
	// Native ETH — zero address
	if (address == common.Address{}) {
		return s.SaveERC20Token(address, "Ether", "ETH", 18)
	}

	parsed, err := abi.JSON(strings.NewReader(erc20LookupABI))
	if err != nil {
		return err
	}

	callStr := func(method string) string {
		data, err := parsed.Pack(method)
		if err != nil {
			return ""
		}
		cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		raw, err := client.CallContract(cctx, ethereum.CallMsg{To: &address, Data: data}, nil)
		if err != nil {
			return ""
		}
		// Try standard ABI string decode first.
		vals, err := parsed.Unpack(method, raw)
		if err == nil && len(vals) > 0 {
			if str, ok := vals[0].(string); ok {
				return strings.TrimSpace(str)
			}
		}
		// Fallback: some old tokens (MKR, SAI) return bytes32 instead of string.
		if len(raw) >= 32 {
			return strings.TrimRight(string(raw[:32]), "\x00")
		}
		return ""
	}

	callUint8 := func(method string) uint8 {
		data, err := parsed.Pack(method)
		if err != nil {
			return 0
		}
		cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		raw, err := client.CallContract(cctx, ethereum.CallMsg{To: &address, Data: data}, nil)
		if err != nil {
			return 0
		}
		vals, err := parsed.Unpack(method, raw)
		if err == nil && len(vals) > 0 {
			if v, ok := vals[0].(uint8); ok {
				return v
			}
		}
		return 0
	}

	name := callStr("name")
	symbol := callStr("symbol")
	decimals := callUint8("decimals")

	return s.SaveERC20Token(address, name, symbol, decimals)
}
