// Package signer manages Ethereum private keys and signs EIP-4527 unsigned
// transactions decoded from UR QR codes. It is the Go counterpart to
// signer/eth_signer.py and shares the same keys file at
// ~/.charm-wallet-private-keys.json.
package signer

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	gocrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// ── Key store ─────────────────────────────────────────────────────────────────

// KeyEntry is one entry in the private-keys file.
type KeyEntry struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	PrivateKey string `json:"private_key"`
}

// KeysFile is the JSON envelope stored at ~/.charm-wallet-private-keys.json.
type KeysFile struct {
	Keys []KeyEntry `json:"private_keys"`
}

const (
	DefaultPrivateKey = "0xc0054fba575ebf91c5bdf3ddbd53a71ace4204e7623057cf95a8a8da7b4a4efc"
	DefaultKeyName    = "NotForProduction"
)

// DefaultAddress returns the Ethereum address derived from DefaultPrivateKey.
func DefaultAddress() string {
	addr, _ := DeriveAddress(DefaultPrivateKey)
	return addr
}

// KeysPath returns the absolute path to the private-keys file.
func KeysPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".charm-wallet-private-keys.json")
}

// ConfigPath returns the absolute path to the main app config.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".charm-wallet-config.json")
}

// DeriveAddress returns the checksummed Ethereum address for a hex private key.
func DeriveAddress(hexKey string) (string, error) {
	stripped := strings.TrimPrefix(hexKey, "0x")
	privKey, err := gocrypto.HexToECDSA(stripped)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}
	return gocrypto.PubkeyToAddress(privKey.PublicKey).Hex(), nil
}

// LoadKeys reads the keys file, bootstrapping defaults on first run.
func LoadKeys() ([]KeyEntry, error) {
	path := KeysPath()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return bootstrapKeys(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading keys file: %w", err)
	}
	var kf KeysFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, fmt.Errorf("parsing keys file: %w", err)
	}
	return kf.Keys, nil
}

// AddKey appends a new entry (derived address + name) to the keys file.
// Returns an error if a key for the same address already exists.
func AddKey(hexKey, name string) (KeyEntry, error) {
	addr, err := DeriveAddress(hexKey)
	if err != nil {
		return KeyEntry{}, err
	}
	keys, err := LoadKeys()
	if err != nil {
		return KeyEntry{}, err
	}
	for _, k := range keys {
		if strings.EqualFold(k.Address, addr) {
			return KeyEntry{}, fmt.Errorf("key for %s already exists", addr)
		}
	}
	entry := KeyEntry{Address: addr, Name: name, PrivateKey: hexKey}
	keys = append(keys, entry)
	if err := saveKeys(keys); err != nil {
		return KeyEntry{}, err
	}
	_ = registerWallet(addr, name)
	return entry, nil
}

// FindKey returns the private key for address, or "" if not found.
func FindKey(address string, keys []KeyEntry) string {
	for _, k := range keys {
		if strings.EqualFold(k.Address, address) {
			return k.PrivateKey
		}
	}
	return ""
}

func bootstrapKeys(path string) ([]KeyEntry, error) {
	addr, err := DeriveAddress(DefaultPrivateKey)
	if err != nil {
		return nil, err
	}
	entry := KeyEntry{Address: addr, Name: DefaultKeyName, PrivateKey: DefaultPrivateKey}
	keys := []KeyEntry{entry}
	if err := saveKeys(keys); err != nil {
		return nil, err
	}
	_ = registerWallet(addr, DefaultKeyName)
	return keys, nil
}

func saveKeys(keys []KeyEntry) error {
	path := KeysPath()
	data, err := json.MarshalIndent(KeysFile{Keys: keys}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// registerWallet adds the address to the main app config if absent.
func registerWallet(address, name string) error {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	var wallets []map[string]interface{}
	if raw, ok := cfg["wallets"]; ok {
		_ = json.Unmarshal(raw, &wallets)
	}
	for _, w := range wallets {
		if addr, ok := w["address"].(string); ok && strings.EqualFold(addr, address) {
			return nil
		}
	}
	wallets = append(wallets, map[string]interface{}{
		"address": address,
		"name":    name,
		"active":  false,
	})
	raw, _ := json.Marshal(wallets)
	cfg["wallets"] = json.RawMessage(raw)
	out, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, out, 0644)
}

// ── Bytewords ─────────────────────────────────────────────────────────────────
// Mirrors the bytewordsLookup table in rpc/rpc.go.

var bwEncode = [512]byte{
	'a', 'e', 'a', 'd', 'a', 'o', 'a', 'x', 'a', 'a', 'a', 'h', 'a', 'm', 'a', 't', // 0-7
	'a', 'y', 'a', 's', 'b', 'k', 'b', 'd', 'b', 'n', 'b', 't', 'b', 'a', 'b', 's', // 8-15
	'b', 'e', 'b', 'y', 'b', 'g', 'b', 'w', 'b', 'b', 'b', 'z', 'c', 'm', 'c', 'h', // 16-23
	'c', 's', 'c', 'f', 'c', 'y', 'c', 'w', 'c', 'e', 'c', 'a', 'c', 'k', 'c', 't', // 24-31
	'c', 'x', 'c', 'l', 'c', 'p', 'c', 'n', 'd', 'k', 'd', 'a', 'd', 's', 'd', 'i', // 32-39
	'd', 'e', 'd', 't', 'd', 'r', 'd', 'n', 'd', 'w', 'd', 'p', 'd', 'm', 'd', 'l', // 40-47
	'd', 'y', 'e', 'h', 'e', 'y', 'e', 'o', 'e', 'e', 'e', 'c', 'e', 'n', 'e', 'm', // 48-55
	'e', 't', 'e', 's', 'f', 't', 'f', 'r', 'f', 'n', 'f', 's', 'f', 'm', 'f', 'h', // 56-63
	'f', 'z', 'f', 'p', 'f', 'w', 'f', 'x', 'f', 'y', 'f', 'e', 'f', 'g', 'f', 'l', // 64-71
	'f', 'd', 'g', 'a', 'g', 'e', 'g', 'r', 'g', 's', 'g', 't', 'g', 'l', 'g', 'w', // 72-79
	'g', 'd', 'g', 'y', 'g', 'm', 'g', 'u', 'g', 'h', 'g', 'o', 'h', 'f', 'h', 'g', // 80-87
	'h', 'd', 'h', 'k', 'h', 't', 'h', 'p', 'h', 'h', 'h', 'l', 'h', 'y', 'h', 'e', // 88-95
	'h', 'n', 'h', 's', 'i', 'd', 'i', 'a', 'i', 'e', 'i', 'h', 'i', 'y', 'i', 'o', // 96-103
	'i', 's', 'i', 'n', 'i', 'm', 'j', 'e', 'j', 'z', 'j', 'n', 'j', 't', 'j', 'l', // 104-111
	'j', 'o', 'j', 's', 'j', 'p', 'j', 'k', 'j', 'y', 'k', 'p', 'k', 'o', 'k', 't', // 112-119
	'k', 's', 'k', 'k', 'k', 'n', 'k', 'g', 'k', 'e', 'k', 'i', 'k', 'b', 'l', 'b', // 120-127
	'l', 'a', 'l', 'y', 'l', 'f', 'l', 's', 'l', 'r', 'l', 'p', 'l', 'n', 'l', 't', // 128-135
	'l', 'o', 'l', 'd', 'l', 'e', 'l', 'u', 'l', 'k', 'l', 'g', 'm', 'n', 'm', 'y', // 136-143
	'm', 'h', 'm', 'e', 'm', 'o', 'm', 'u', 'm', 'w', 'm', 'd', 'm', 't', 'm', 's', // 144-151
	'm', 'k', 'n', 'l', 'n', 'y', 'n', 'd', 'n', 'w', 'n', 't', 'n', 'n', 'n', 'e', // 152-159
	'n', 'b', 'o', 'y', 'o', 'e', 'o', 't', 'o', 'x', 'o', 'n', 'o', 'l', 'o', 's', // 160-167
	'p', 'd', 'p', 't', 'p', 'k', 'p', 'y', 'p', 's', 'p', 'm', 'p', 'l', 'p', 'e', // 168-175
	'p', 'f', 'p', 'a', 'p', 'r', 'q', 'd', 'q', 'z', 'r', 'e', 'r', 'p', 'r', 'l', // 176-183
	'r', 'o', 'r', 'h', 'r', 'd', 'r', 'k', 'r', 'f', 'r', 'y', 'r', 'n', 'r', 's', // 184-191
	'r', 't', 's', 'e', 's', 'a', 's', 'r', 's', 's', 's', 'k', 's', 'w', 's', 't', // 192-199
	's', 'p', 's', 'o', 's', 'g', 's', 'b', 's', 'f', 's', 'n', 't', 'o', 't', 'k', // 200-207
	't', 'i', 't', 't', 't', 'd', 't', 'e', 't', 'y', 't', 'l', 't', 'b', 't', 's', // 208-215
	't', 'p', 't', 'a', 't', 'n', 'u', 'y', 'u', 'o', 'u', 't', 'u', 'e', 'u', 'r', // 216-223
	'v', 't', 'v', 'y', 'v', 'o', 'v', 'l', 'v', 'e', 'v', 'w', 'v', 'a', 'v', 'd', // 224-231
	'v', 's', 'w', 'l', 'w', 'd', 'w', 'm', 'w', 'p', 'w', 'e', 'w', 'y', 'w', 's', // 232-239
	'w', 't', 'w', 'n', 'w', 'z', 'w', 'f', 'w', 'k', 'y', 'k', 'y', 'n', 'y', 'l', // 240-247
	'y', 'a', 'y', 't', 'z', 's', 'z', 'o', 'z', 't', 'z', 'c', 'z', 'e', 'z', 'm', // 248-255
}

var bwDecode [256][256]int16 // [first][last] → byte value, -1 = invalid

func init() {
	for r := range bwDecode {
		for c := range bwDecode[r] {
			bwDecode[r][c] = -1
		}
	}
	for i := 0; i < 256; i++ {
		f := bwEncode[i*2]
		l := bwEncode[i*2+1]
		bwDecode[f][l] = int16(i)
	}
}

func decodeBytewords(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("bytewords: odd length string")
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		v := bwDecode[s[i]][s[i+1]]
		if v < 0 {
			return nil, fmt.Errorf("bytewords: unknown pair %q at %d", s[i:i+2], i)
		}
		out[i/2] = byte(v)
	}
	return out, nil
}

// ── Minimal CBOR primitives ───────────────────────────────────────────────────
// Only the subset needed to parse our own buildEthSignRequestCBOR output.

// cborLen extracts the count/value from the additional-info bits of a CBOR
// header byte at data[0]. Returns (value, extraBytes, error).
func cborLen(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	ai := data[0] & 0x1F
	switch {
	case ai <= 23:
		return uint64(ai), 0, nil
	case ai == 24:
		if len(data) < 2 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint64(data[1]), 1, nil
	case ai == 25:
		if len(data) < 3 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint64(data[1])<<8 | uint64(data[2]), 2, nil
	case ai == 26:
		if len(data) < 5 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		return uint64(data[1])<<24 | uint64(data[2])<<16 | uint64(data[3])<<8 | uint64(data[4]), 4, nil
	case ai == 27:
		if len(data) < 9 {
			return 0, 0, io.ErrUnexpectedEOF
		}
		var v uint64
		for i := 1; i <= 8; i++ {
			v = v<<8 | uint64(data[i])
		}
		return v, 8, nil
	}
	return 0, 0, fmt.Errorf("cbor: indefinite length not supported (ai=%d)", ai)
}

// cborSkip returns the number of bytes consumed by one CBOR item at data[0:].
func cborSkip(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	mt := data[0] >> 5
	count, extra, err := cborLen(data)
	if err != nil {
		return 0, err
	}
	hdr := 1 + extra
	switch mt {
	case 0, 1: // uint, negint
		return hdr, nil
	case 2, 3: // bytes, text
		return hdr + int(count), nil
	case 4: // array
		pos := hdr
		for i := uint64(0); i < count; i++ {
			n, err := cborSkip(data[pos:])
			if err != nil {
				return 0, err
			}
			pos += n
		}
		return pos, nil
	case 5: // map
		pos := hdr
		for i := uint64(0); i < count*2; i++ {
			n, err := cborSkip(data[pos:])
			if err != nil {
				return 0, err
			}
			pos += n
		}
		return pos, nil
	case 6: // tag — skip tag, then skip value
		n, err := cborSkip(data[hdr:])
		if err != nil {
			return 0, err
		}
		return hdr + n, nil
	}
	return 0, fmt.Errorf("cbor: unsupported major type %d", mt)
}

// cborReadUint reads a CBOR unsigned integer at data[0:].
func cborReadUint(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	if data[0]>>5 != 0 {
		return 0, 0, fmt.Errorf("cbor: expected uint (MT=0), got MT=%d", data[0]>>5)
	}
	v, extra, err := cborLen(data)
	return v, 1 + extra, err
}

// cborReadBytes reads a CBOR byte string at data[0:].
func cborReadBytes(data []byte) ([]byte, int, error) {
	if len(data) == 0 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	if data[0]>>5 != 2 {
		return nil, 0, fmt.Errorf("cbor: expected bytes (MT=2), got MT=%d", data[0]>>5)
	}
	length, extra, err := cborLen(data)
	if err != nil {
		return nil, 0, err
	}
	hdr := 1 + extra
	end := hdr + int(length)
	if len(data) < end {
		return nil, 0, io.ErrUnexpectedEOF
	}
	return data[hdr:end], end, nil
}

// parseEthSignRequest extracts (rlpBytes, chainID, fromAddress) from the CBOR
// payload produced by buildEthSignRequestCBOR.
func parseEthSignRequest(data []byte) (rlpBytes []byte, chainID uint64, fromBytes []byte, err error) {
	// First byte must be map(5) = 0xA5
	if len(data) < 1 || data[0] != 0xA5 {
		return nil, 0, nil, fmt.Errorf("expected CBOR map(5), got %02x", data[0])
	}
	pos := 1
	for i := 0; i < 5; i++ {
		var key uint64
		var n int
		key, n, err = cborReadUint(data[pos:])
		if err != nil {
			return nil, 0, nil, fmt.Errorf("map key %d: %w", i, err)
		}
		pos += n
		switch key {
		case 1: // tag(37)+bytes(16) UUID — skip
			n, err = cborSkip(data[pos:])
		case 2: // RLP bytes
			rlpBytes, n, err = cborReadBytes(data[pos:])
		case 3: // data-type uint — skip
			n, err = cborSkip(data[pos:])
		case 4: // chainId
			chainID, n, err = cborReadUint(data[pos:])
		case 6: // from address bytes
			fromBytes, n, err = cborReadBytes(data[pos:])
		default:
			n, err = cborSkip(data[pos:])
		}
		if err != nil {
			return nil, 0, nil, fmt.Errorf("map value for key %d: %w", key, err)
		}
		pos += n
	}
	return rlpBytes, chainID, fromBytes, nil
}

// ── EIP-4527 decode ───────────────────────────────────────────────────────────

// txPreimage mirrors the RLP structure used in rpc/rpc.go.
// rlp:"optional" avoids errors on empty trailing zero-value fields.
type txPreimage struct {
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	To       common.Address
	Value    *big.Int
	Data     []byte
	V        *big.Int // ChainID (EIP-155)
	R        *big.Int // 0 for unsigned
	S        *big.Int // 0 for unsigned
}

// DecodedTx holds the fields extracted from an EIP-4527 UR.
type DecodedTx struct {
	From     string
	To       string
	Nonce    uint64
	GasPrice *big.Int
	Gas      uint64
	Value    *big.Int
	Data     []byte
	ChainID  *big.Int
}

// DecodeEIP4527UR parses a ur:eth-sign-request/... string into a DecodedTx.
//
// Pipeline: bytewords → CRC32 verify → CBOR → RLP decode
func DecodeEIP4527UR(ur string) (DecodedTx, error) {
	const prefix = "ur:eth-sign-request/"
	if !strings.HasPrefix(strings.ToLower(ur), prefix) {
		return DecodedTx{}, errors.New("not a ur:eth-sign-request UR")
	}

	payload, err := decodeBytewords(ur[len(prefix):])
	if err != nil {
		return DecodedTx{}, fmt.Errorf("bytewords decode: %w", err)
	}
	if len(payload) < 5 {
		return DecodedTx{}, errors.New("payload too short")
	}

	// Verify CRC32 (last 4 bytes, big-endian IEEE)
	cborData := payload[:len(payload)-4]
	expected := uint32(payload[len(payload)-4])<<24 |
		uint32(payload[len(payload)-3])<<16 |
		uint32(payload[len(payload)-2])<<8 |
		uint32(payload[len(payload)-1])
	if actual := crc32IEEE(cborData); actual != expected {
		return DecodedTx{}, fmt.Errorf("CRC32 mismatch: expected %08x got %08x", expected, actual)
	}

	rlpBytes, chainID, fromBytes, err := parseEthSignRequest(cborData)
	if err != nil {
		return DecodedTx{}, fmt.Errorf("CBOR: %w", err)
	}

	var pre txPreimage
	if err := rlp.DecodeBytes(rlpBytes, &pre); err != nil {
		return DecodedTx{}, fmt.Errorf("RLP decode: %w", err)
	}

	effectiveChain := pre.V // V field holds chainID in unsigned preimage
	if effectiveChain == nil || effectiveChain.Sign() == 0 {
		effectiveChain = new(big.Int).SetUint64(chainID)
	}

	var fromAddr string
	if len(fromBytes) == 20 {
		fromAddr = common.BytesToAddress(fromBytes).Hex()
	}

	return DecodedTx{
		From:     fromAddr,
		To:       pre.To.Hex(),
		Nonce:    pre.Nonce,
		GasPrice: pre.GasPrice,
		Gas:      pre.Gas,
		Value:    pre.Value,
		Data:     pre.Data,
		ChainID:  effectiveChain,
	}, nil
}

// crc32IEEE computes the CRC32 IEEE checksum, matching hash/crc32.ChecksumIEEE.
// We replicate it here to avoid an import solely for this constant.
func crc32IEEE(data []byte) uint32 {
	// Standard IEEE polynomial
	const poly = 0xedb88320
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}

// ── Transaction signing ───────────────────────────────────────────────────────

// SignResult contains the output of a successful signing operation.
type SignResult struct {
	RawTx      string // "0x..." RLP-encoded signed transaction
	TxHash     string // "0x..." keccak256 of signed tx
	R, S       string // signature components (hex)
	V          string
	From       string
	To         string
	ValueHuman string // human-readable value e.g. "0.001 ETH"
}

// SignTx signs a DecodedTx with the given hex private key and returns the result.
func SignTx(tx DecodedTx, hexKey string) (SignResult, error) {
	stripped := strings.TrimPrefix(hexKey, "0x")
	privKey, err := gocrypto.HexToECDSA(stripped)
	if err != nil {
		return SignResult{}, fmt.Errorf("invalid private key: %w", err)
	}

	toAddr := common.HexToAddress(tx.To)
	legacyTx := &types.LegacyTx{
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		Gas:      tx.Gas,
		To:       &toAddr,
		Value:    tx.Value,
		Data:     tx.Data,
	}
	unsigned := types.NewTx(legacyTx)
	signer := types.NewEIP155Signer(tx.ChainID)
	signed, err := types.SignTx(unsigned, signer, privKey)
	if err != nil {
		return SignResult{}, fmt.Errorf("signing: %w", err)
	}

	raw, err := signed.MarshalBinary()
	if err != nil {
		return SignResult{}, fmt.Errorf("marshal: %w", err)
	}

	sigV, sigR, sigS := signed.RawSignatureValues()

	return SignResult{
		RawTx:      "0x" + hex.EncodeToString(raw),
		TxHash:     signed.Hash().Hex(),
		R:          fmt.Sprintf("0x%x", sigR),
		S:          fmt.Sprintf("0x%x", sigS),
		V:          fmt.Sprintf("0x%x", sigV),
		From:       tx.From,
		To:         tx.To,
		ValueHuman: WeiToEthStr(tx.Value),
	}, nil
}

// WeiToEthStr formats a wei amount as an ETH string (up to 6 decimal places).
func WeiToEthStr(wei *big.Int) string {
	if wei == nil || wei.Sign() == 0 {
		return "0 ETH"
	}
	eth := new(big.Float).Quo(
		new(big.Float).SetInt(wei),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
	)
	return eth.Text('f', 6) + " ETH"
}
