package store

import (
	"database/sql"
	"math/big"

	"charm-wallet-tui/indexer"

	"github.com/ethereum/go-ethereum/common"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS indexed_events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	block      INTEGER NOT NULL,
	tx_hash    TEXT    NOT NULL,
	log_index  INTEGER NOT NULL,
	from_addr  TEXT    NOT NULL,
	to_addr    TEXT    NOT NULL,
	value_hex  TEXT    NOT NULL,
	token_addr TEXT    NOT NULL,
	symbol     TEXT    NOT NULL,
	decimals   INTEGER NOT NULL,
	seen_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tx_hash, log_index)
);
CREATE INDEX IF NOT EXISTS idx_from  ON indexed_events(from_addr);
CREATE INDEX IF NOT EXISTS idx_to    ON indexed_events(to_addr);
CREATE INDEX IF NOT EXISTS idx_block ON indexed_events(block);

CREATE TABLE IF NOT EXISTS v4_swap_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	block       INTEGER NOT NULL,
	tx_hash     TEXT    NOT NULL,
	log_index   INTEGER NOT NULL,
	pool_id     TEXT    NOT NULL,
	sender      TEXT    NOT NULL,
	amount0     TEXT    NOT NULL,
	amount1     TEXT    NOT NULL,
	sqrt_price  TEXT    NOT NULL,
	liquidity   TEXT    NOT NULL,
	tick        INTEGER NOT NULL,
	fee         INTEGER NOT NULL,
	seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tx_hash, log_index)
);
CREATE INDEX IF NOT EXISTS idx_v4_sender ON v4_swap_events(sender);
CREATE INDEX IF NOT EXISTS idx_v4_block  ON v4_swap_events(block);
CREATE INDEX IF NOT EXISTS idx_v4_pool   ON v4_swap_events(pool_id);
`

// Store wraps a SQLite database for persisting indexed events.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveEvent inserts an indexed event. Silently ignores duplicate (tx_hash, log_index) pairs.
func (s *Store) SaveEvent(ev indexer.IndexedEvent) error {
	valueHex := "0x0"
	if ev.Value != nil {
		valueHex = "0x" + ev.Value.Text(16)
	}
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO indexed_events
			(block, tx_hash, log_index, from_addr, to_addr, value_hex, token_addr, symbol, decimals)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Block,
		ev.TxHash.Hex(),
		ev.LogIndex,
		ev.From.Hex(),
		ev.To.Hex(),
		valueHex,
		ev.Token.Hex(),
		ev.Symbol,
		ev.Decimals,
	)
	return err
}

// SaveV4Swap inserts a Uniswap V4 Swap event. Silently ignores duplicate (tx_hash, log_index) pairs.
func (s *Store) SaveV4Swap(ev indexer.V4SwapEvent) error {
	bigStr := func(x *big.Int) string {
		if x == nil {
			return "0"
		}
		return x.String()
	}
	tick := int64(0)
	if ev.Tick != nil {
		tick = ev.Tick.Int64()
	}
	fee := int64(0)
	if ev.Fee != nil {
		fee = ev.Fee.Int64()
	}
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO v4_swap_events
			(block, tx_hash, log_index, pool_id, sender, amount0, amount1, sqrt_price, liquidity, tick, fee)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.Block,
		ev.TxHash.Hex(),
		ev.LogIndex,
		ev.PoolID.Hex(),
		ev.Sender.Hex(),
		bigStr(ev.Amount0),
		bigStr(ev.Amount1),
		bigStr(ev.SqrtPriceX96),
		bigStr(ev.Liquidity),
		tick,
		fee,
	)
	return err
}

// RecentV4Swaps returns up to limit V4 Swap events ordered newest block first.
func (s *Store) RecentV4Swaps(limit int) ([]indexer.V4SwapEvent, error) {
	rows, err := s.db.Query(`
		SELECT block, tx_hash, log_index, pool_id, sender, amount0, amount1, sqrt_price, liquidity, tick, fee
		FROM v4_swap_events
		ORDER BY block DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []indexer.V4SwapEvent
	for rows.Next() {
		var (
			block     uint64
			txHash    string
			logIdx    uint
			poolID    string
			sender    string
			amount0   string
			amount1   string
			sqrtPrice string
			liquidity string
			tick      int64
			fee       int64
		)
		if err := rows.Scan(&block, &txHash, &logIdx, &poolID, &sender, &amount0, &amount1, &sqrtPrice, &liquidity, &tick, &fee); err != nil {
			continue
		}
		parseBig := func(s string) *big.Int {
			x := new(big.Int)
			x.SetString(s, 10)
			return x
		}
		events = append(events, indexer.V4SwapEvent{
			Block:        block,
			TxHash:       common.HexToHash(txHash),
			LogIndex:     logIdx,
			PoolID:       common.HexToHash(poolID),
			Sender:       common.HexToAddress(sender),
			Amount0:      parseBig(amount0),
			Amount1:      parseBig(amount1),
			SqrtPriceX96: parseBig(sqrtPrice),
			Liquidity:    parseBig(liquidity),
			Tick:         big.NewInt(tick),
			Fee:          big.NewInt(fee),
		})
	}
	return events, rows.Err()
}

// RecentEvents returns up to limit events ordered newest block first.
func (s *Store) RecentEvents(limit int) ([]indexer.IndexedEvent, error) {
	rows, err := s.db.Query(`
		SELECT block, tx_hash, log_index, from_addr, to_addr, value_hex, token_addr, symbol, decimals
		FROM indexed_events
		ORDER BY block DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []indexer.IndexedEvent
	for rows.Next() {
		var (
			block    uint64
			txHash   string
			logIdx   uint
			from     string
			to       string
			valueHex string
			token    string
			symbol   string
			decimals uint8
		)
		if err := rows.Scan(&block, &txHash, &logIdx, &from, &to, &valueHex, &token, &symbol, &decimals); err != nil {
			continue
		}
		value := new(big.Int)
		value.SetString(valueHex, 0) // handles "0x..." prefix

		events = append(events, indexer.IndexedEvent{
			Block:    block,
			TxHash:   common.HexToHash(txHash),
			LogIndex: logIdx,
			From:     common.HexToAddress(from),
			To:       common.HexToAddress(to),
			Value:    value,
			Token:    common.HexToAddress(token),
			Symbol:   symbol,
			Decimals: decimals,
		})
	}
	return events, rows.Err()
}

// Count returns the total number of stored events.
func (s *Store) Count() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM indexed_events`).Scan(&n)
	return n, err
}

// OldestBlock returns the lowest block number stored, or 0 if empty.
func (s *Store) OldestBlock() (uint64, error) {
	var b sql.NullInt64
	err := s.db.QueryRow(`SELECT MIN(block) FROM indexed_events`).Scan(&b)
	if err != nil || !b.Valid {
		return 0, err
	}
	return uint64(b.Int64), nil
}

// LatestBlock returns the highest block number stored, or 0 if empty.
func (s *Store) LatestBlock() (uint64, error) {
	var b sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(block) FROM indexed_events`).Scan(&b)
	if err != nil || !b.Valid {
		return 0, err
	}
	return uint64(b.Int64), nil
}

