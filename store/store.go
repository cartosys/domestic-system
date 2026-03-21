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

CREATE TABLE IF NOT EXISTS v4_pool_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	kind        TEXT    NOT NULL,
	block       INTEGER NOT NULL,
	tx_hash     TEXT    NOT NULL,
	log_index   INTEGER NOT NULL,
	pool_id     TEXT,
	sender      TEXT,
	amount0     TEXT,
	amount1     TEXT,
	sqrt_price  TEXT,
	liquidity   TEXT,
	tick        INTEGER,
	fee         INTEGER,
	tick_lower  INTEGER,
	tick_upper  INTEGER,
	liq_delta   TEXT,
	salt        TEXT,
	from_addr   TEXT,
	to_addr     TEXT,
	token_id    TEXT,
	seen_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(tx_hash, log_index)
);
CREATE INDEX IF NOT EXISTS idx_v4pe_sender ON v4_pool_events(sender);
CREATE INDEX IF NOT EXISTS idx_v4pe_block  ON v4_pool_events(block);
CREATE INDEX IF NOT EXISTS idx_v4pe_pool   ON v4_pool_events(pool_id);
CREATE INDEX IF NOT EXISTS idx_v4pe_from   ON v4_pool_events(from_addr);
CREATE INDEX IF NOT EXISTS idx_v4pe_to     ON v4_pool_events(to_addr);
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


// SaveV4PoolEvent inserts any V4 PoolManager event. Silently ignores duplicate (tx_hash, log_index).
func (s *Store) SaveV4PoolEvent(ev indexer.V4PoolEvent) error {
	bigStr := func(x *big.Int) *string {
		if x == nil {
			return nil
		}
		v := x.String()
		return &v
	}
	nullInt := func(x *big.Int) *int64 {
		if x == nil {
			return nil
		}
		v := x.Int64()
		return &v
	}
	nullStr := func(h common.Hash) *string {
		z := common.Hash{}
		if h == z {
			return nil
		}
		v := h.Hex()
		return &v
	}
	nullAddr := func(a common.Address) *string {
		z := common.Address{}
		if a == z {
			return nil
		}
		v := a.Hex()
		return &v
	}
	var poolID, sender *string
	if ev.Kind != indexer.V4KindTransfer {
		poolID = nullStr(ev.PoolID)
		v := ev.Sender.Hex()
		sender = &v
	}
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO v4_pool_events
			(kind, block, tx_hash, log_index,
			 pool_id, sender, amount0, amount1, sqrt_price, liquidity, tick, fee,
			 tick_lower, tick_upper, liq_delta, salt,
			 from_addr, to_addr, token_id)
		VALUES (?,?,?,?, ?,?,?,?,?,?,?,?, ?,?,?,?, ?,?,?)`,
		ev.Kind.String(), ev.Block, ev.TxHash.Hex(), ev.LogIndex,
		poolID, sender, bigStr(ev.Amount0), bigStr(ev.Amount1),
		bigStr(ev.SqrtPriceX96), bigStr(ev.Liquidity),
		nullInt(ev.Tick), nullInt(ev.Fee),
		nullInt(ev.TickLower), nullInt(ev.TickUpper),
		bigStr(ev.LiquidityDelta), nullStr(ev.Salt),
		nullAddr(ev.From), nullAddr(ev.To), bigStr(ev.TokenID),
	)
	return err
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

