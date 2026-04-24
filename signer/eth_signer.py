#!/usr/bin/env python3
"""
eth_signer.py — Ethereum ECDSA key management and EIP-4527 transaction signing CLI.

Modes:
  --generate-keys                       Generate a new ECDSA keypair
  --derive-address PRIVATE_KEY          Derive address from private key
  --private-key PK --to ADDR --value N  Sign a transaction (wei)
  --scan                                Webcam: detect & sign EIP-4527 QR codes
  --add-key --private-key PK --name N   Add a key to the keys file
  --list-keys                           List stored addresses (no secrets shown)

Private keys file: ~/.charm-wallet-private-keys.json  (mode 600, outside repo)
Main app config:   ~/.charm-wallet-config.json
"""

import argparse
import json
import os
import secrets
import struct
import sys
import zlib
from pathlib import Path
from typing import Optional

# ── Third-party (required) ────────────────────────────────────────────────────
try:
    from eth_account import Account
except ImportError:
    sys.exit(json.dumps({"error": "eth-account not installed. Run: pip install eth-account"}))

# ── Paths ─────────────────────────────────────────────────────────────────────

KEYS_FILE   = Path.home() / ".charm-wallet-private-keys.json"
CONFIG_FILE = Path.home() / ".charm-wallet-config.json"

_DEFAULT_PRIVATE_KEY = "0xc0054fba575ebf91c5bdf3ddbd53a71ace4204e7623057cf95a8a8da7b4a4efc"
_DEFAULT_KEY_NAME    = "NotForProduction"

# ── Bytewords ─────────────────────────────────────────────────────────────────
# Blockchain Commons bc-bytewords minimal encoding: first + last char of each
# 4-letter word, concatenated.  Table mirrors rpc/rpc.go bytewordsLookup.

_BW_ENC = [
    'ae','ad','ao','ax','aa','ah','am','at',  # 0-7
    'ay','as','bk','bd','bn','bt','ba','bs',  # 8-15
    'be','by','bg','bw','bb','bz','cm','ch',  # 16-23
    'cs','cf','cy','cw','ce','ca','ck','ct',  # 24-31
    'cx','cl','cp','cn','dk','da','ds','di',  # 32-39
    'de','dt','dr','dn','dw','dp','dm','dl',  # 40-47
    'dy','eh','ey','eo','ee','ec','en','em',  # 48-55
    'et','es','ft','fr','fn','fs','fm','fh',  # 56-63
    'fz','fp','fw','fx','fy','fe','fg','fl',  # 64-71
    'fd','ga','ge','gr','gs','gt','gl','gw',  # 72-79
    'gd','gy','gm','gu','gh','go','hf','hg',  # 80-87
    'hd','hk','ht','hp','hh','hl','hy','he',  # 88-95
    'hn','hs','id','ia','ie','ih','iy','io',  # 96-103
    'is','in','im','je','jz','jn','jt','jl',  # 104-111
    'jo','js','jp','jk','jy','kp','ko','kt',  # 112-119
    'ks','kk','kn','kg','ke','ki','kb','lb',  # 120-127
    'la','ly','lf','ls','lr','lp','ln','lt',  # 128-135
    'lo','ld','le','lu','lk','lg','mn','my',  # 136-143
    'mh','me','mo','mu','mw','md','mt','ms',  # 144-151
    'mk','nl','ny','nd','nw','nt','nn','ne',  # 152-159
    'nb','oy','oe','ot','ox','on','ol','os',  # 160-167
    'pd','pt','pk','py','ps','pm','pl','pe',  # 168-175
    'pf','pa','pr','qd','qz','re','rp','rl',  # 176-183
    'ro','rh','rd','rk','rf','ry','rn','rs',  # 184-191
    'rt','se','sa','sr','ss','sk','sw','st',  # 192-199
    'sp','so','sg','sb','sf','sn','to','tk',  # 200-207
    'ti','tt','td','te','ty','tl','tb','ts',  # 208-215
    'tp','ta','tn','uy','uo','ut','ue','ur',  # 216-223
    'vt','vy','vo','vl','ve','vw','va','vd',  # 224-231
    'vs','wl','wd','wm','wp','we','wy','ws',  # 232-239
    'wt','wn','wz','wf','wk','yk','yn','yl',  # 240-247
    'ya','yt','zs','zo','zt','zc','ze','zm',  # 248-255
]
_BW_DEC: dict = {v: i for i, v in enumerate(_BW_ENC)}


def _bytewords_decode(s: str) -> bytes:
    if len(s) % 2 != 0:
        raise ValueError("Bytewords string must have even length")
    out = bytearray()
    for i in range(0, len(s), 2):
        pair = s[i:i+2]
        if pair not in _BW_DEC:
            raise ValueError(f"Unknown bytewords pair at {i}: {pair!r}")
        out.append(_BW_DEC[pair])
    return bytes(out)


# ── Minimal RLP decoder ───────────────────────────────────────────────────────
# Decodes the EIP-155 unsigned tx signing preimage without external deps.
# Returns bytes for scalars, list[bytes] for sequences.

def _rlp_one(data: bytes) -> tuple:
    """Return (value, remaining) where value is bytes or list."""
    if not data:
        return b'', b''
    b = data[0]
    if b < 0x80:                        # single byte
        return bytes([b]), data[1:]
    elif b <= 0xB7:                     # short byte string
        n = b - 0x80
        return data[1:1+n], data[1+n:]
    elif b <= 0xBF:                     # long byte string
        ll = b - 0xB7
        n = int.from_bytes(data[1:1+ll], 'big')
        s = 1 + ll
        return data[s:s+n], data[s+n:]
    elif b <= 0xF7:                     # short list
        n = b - 0xC0
        return _rlp_list(data[1:1+n]), data[1+n:]
    else:                               # long list
        ll = b - 0xF7
        n = int.from_bytes(data[1:1+ll], 'big')
        s = 1 + ll
        return _rlp_list(data[s:s+n]), data[s+n:]


def _rlp_list(data: bytes) -> list:
    items = []
    while data:
        item, data = _rlp_one(data)
        items.append(item)
    return items


def _rlp_decode_tx(data: bytes) -> list:
    """Decode an RLP-encoded Ethereum transaction list → list[bytes]."""
    result, _ = _rlp_one(data)
    if not isinstance(result, list):
        raise ValueError("RLP payload is not a list")
    return result


def _be_int(b: bytes) -> int:
    """Big-endian bytes → int (empty bytes = 0)."""
    return int.from_bytes(b, 'big') if b else 0


# ── EIP-4527 UR decoder ───────────────────────────────────────────────────────

def decode_eip4527_ur(ur: str) -> dict:
    """
    Decode a UR eth-sign-request string into a transaction dict.

    Pipeline:  bytewords → bytes → CRC32 verify → CBOR → RLP → tx dict
    Returns keys: from, to, nonce, gasPrice, gas, value, data, chainId
    """
    try:
        import cbor2
    except ImportError:
        raise RuntimeError("cbor2 not installed. Run: pip install cbor2")

    prefix = "ur:eth-sign-request/"
    if not ur.lower().startswith(prefix):
        raise ValueError("Not a ur:eth-sign-request UR")

    payload = _bytewords_decode(ur[len(prefix):])

    if len(payload) < 5:
        raise ValueError("Payload too short to contain CRC")

    # Last 4 bytes = CRC32 checksum of the CBOR data (big-endian, IEEE)
    cbor_data   = payload[:-4]
    expected    = struct.unpack('>I', payload[-4:])[0]
    actual      = zlib.crc32(cbor_data) & 0xFFFFFFFF
    if actual != expected:
        raise ValueError(f"CRC32 mismatch: expected {expected:#010x}, got {actual:#010x}")

    # CBOR map with integer keys (1-6, no 5):
    #   1 → tag(37, uuid bytes)   request-id
    #   2 → bytes                 RLP unsigned tx
    #   3 → uint(1)               data-type = transaction
    #   4 → uint                  chain-id
    #   6 → bytes(20)             from address
    cbor_map = cbor2.loads(cbor_data)

    rlp_bytes  = cbor_map[2]
    chain_id   = int(cbor_map.get(4, 1))
    from_bytes = cbor_map.get(6, b'')
    from_addr  = ('0x' + from_bytes.hex()) if from_bytes else None

    # RLP: [nonce, gasPrice, gasLimit, to, value, data, chainId, 0, 0]
    fields = _rlp_decode_tx(rlp_bytes)
    if len(fields) < 9:
        raise ValueError(f"Expected 9 RLP fields, got {len(fields)}")

    to_bytes = fields[3]
    to_addr  = ('0x' + to_bytes.hex()) if to_bytes else None
    data_hex = ('0x' + fields[5].hex()) if fields[5] else '0x'

    return {
        'from':     from_addr,
        'to':       to_addr,
        'nonce':    _be_int(fields[0]),
        'gasPrice': _be_int(fields[1]),
        'gas':      _be_int(fields[2]),
        'value':    _be_int(fields[4]),
        'data':     data_hex,
        'chainId':  _be_int(fields[6]) or chain_id,
    }


# ── ECDSA helpers ─────────────────────────────────────────────────────────────

def derive_address(private_key: str) -> str:
    """Derive the Ethereum address from a private key via ECDSA (secp256k1)."""
    return Account.from_key(private_key).address


# ── Private key file management ───────────────────────────────────────────────

def _write_keys(data: dict) -> None:
    KEYS_FILE.parent.mkdir(parents=True, exist_ok=True)
    KEYS_FILE.write_text(json.dumps(data, indent=2))
    os.chmod(KEYS_FILE, 0o600)


def _register_in_config(address: str, name: str) -> None:
    """Add wallet to ~/.charm-wallet-config.json if absent."""
    if not CONFIG_FILE.exists():
        return
    try:
        cfg = json.loads(CONFIG_FILE.read_text())
    except Exception:
        return
    wallets = cfg.get('wallets', [])
    if any(w.get('address', '').lower() == address.lower() for w in wallets):
        return
    wallets.append({"address": address, "name": name, "active": False})
    cfg['wallets'] = wallets
    CONFIG_FILE.write_text(json.dumps(cfg, indent=2))


def _bootstrap_keys() -> dict:
    """Create the default keys file with the NotForProduction key."""
    address = derive_address(_DEFAULT_PRIVATE_KEY)
    data = {
        "private_keys": [
            {
                "address":     address,
                "name":        _DEFAULT_KEY_NAME,
                "private_key": _DEFAULT_PRIVATE_KEY,
            }
        ]
    }
    _write_keys(data)
    _register_in_config(address, _DEFAULT_KEY_NAME)
    _warn({
        "info":    "Created default keys file",
        "path":    str(KEYS_FILE),
        "warning": "Default key is NOT for production — replace it",
        "address": address,
    })
    return data


def load_keys() -> dict:
    """Return keys file contents, bootstrapping defaults if absent."""
    if KEYS_FILE.exists():
        try:
            return json.loads(KEYS_FILE.read_text())
        except Exception as e:
            raise RuntimeError(f"Cannot parse {KEYS_FILE}: {e}")
    return _bootstrap_keys()


def find_key(address: str) -> Optional[str]:
    """Return the private key for the given address, or None."""
    addr_lc = address.lower()
    for entry in load_keys().get('private_keys', []):
        if entry.get('address', '').lower() == addr_lc:
            return entry['private_key']
    return None


def cmd_add_key(private_key: str, name: str) -> dict:
    """Add a new private key to the keys file."""
    address = derive_address(private_key)
    data    = load_keys()
    entries = data.get('private_keys', [])
    if any(e.get('address', '').lower() == address.lower() for e in entries):
        return {"error": f"Key for {address} already exists"}
    entries.append({"address": address, "name": name, "private_key": private_key})
    data['private_keys'] = entries
    _write_keys(data)
    _register_in_config(address, name)
    return {"added": True, "address": address, "name": name}


# ── Transaction operations ────────────────────────────────────────────────────

def cmd_generate_keys() -> dict:
    """Generate a cryptographically secure ECDSA keypair."""
    pk      = '0x' + secrets.token_hex(32)
    address = derive_address(pk)
    return {"private_key": pk, "public_key": address}


def _sign_tx(private_key: str, to: str, value: int,
             nonce: int, gas_price: int, gas_limit: int,
             chain_id: int, data: bytes) -> dict:
    """Core signing function — returns full JSON result."""
    account = Account.from_key(private_key)
    tx = {
        'nonce':    nonce,
        'gasPrice': gas_price,
        'gas':      gas_limit,
        'to':       to,
        'value':    value,
        'data':     data,
        'chainId':  chain_id,
    }
    signed = Account.sign_transaction(tx, private_key)
    return {
        "transaction": {
            "from":     account.address,
            "to":       to,
            "value":    str(value),
            "nonce":    nonce,
            "gas":      gas_limit,
            "gasPrice": gas_price,
            "chainId":  chain_id,
        },
        "signature": {
            "r": hex(signed.r),
            "s": hex(signed.s),
            "v": hex(signed.v),
        },
        "raw_transaction": signed.rawTransaction.hex(),
    }


def cmd_sign(private_key: str, to: str, value: int,
             nonce: int = 0, gas_price: int = 20_000_000_000,
             gas_limit: int = 21_000, chain_id: int = 1,
             data_hex: str = '0x') -> dict:
    """Sign a transaction from CLI parameters."""
    data = bytes.fromhex(data_hex[2:] if data_hex.startswith('0x') else data_hex)
    return _sign_tx(private_key, to, value, nonce, gas_price, gas_limit, chain_id, data)


def _sign_eip4527(tx: dict) -> dict:
    """Sign a decoded EIP-4527 tx dict using a stored key for tx['from']."""
    from_addr = tx.get('from')
    if not from_addr:
        return {"error": "No 'from' address in decoded transaction"}

    pk = find_key(from_addr)
    if pk is None:
        available = [e['address'] for e in load_keys().get('private_keys', [])]
        return {
            "error":               f"No private key stored for {from_addr}",
            "available_addresses": available,
        }

    data_hex = tx.get('data', '0x')
    data     = bytes.fromhex(data_hex[2:] if data_hex.startswith('0x') else data_hex)

    return _sign_tx(
        private_key=pk,
        to=tx['to'],
        value=tx['value'],
        nonce=tx['nonce'],
        gas_price=tx['gasPrice'],
        gas_limit=tx['gas'],
        chain_id=tx['chainId'],
        data=data,
    )


# ── Webcam scan mode ──────────────────────────────────────────────────────────

def cmd_scan() -> None:
    """
    Open the webcam and scan for EIP-4527 QR codes.
    When a matching key is found the signed transaction is emitted as JSON to stdout.
    Press 'q' to quit.
    """
    try:
        import cv2
    except ImportError:
        _die("opencv-python not installed. Run: pip install opencv-python")

    load_keys()  # bootstrap defaults before opening camera

    cap = cv2.VideoCapture(0)
    if not cap.isOpened():
        _die("Cannot open webcam")

    detector     = cv2.QRCodeDetector()
    last_ur      = None
    _warn({"status": "Scanning for EIP-4527 QR codes", "hint": "Press 'q' to quit"})

    while True:
        ret, frame = cap.read()
        if not ret:
            break

        qr_data, pts, _ = detector.detectAndDecode(frame)

        if qr_data and qr_data != last_ur:
            last_ur = qr_data

            if pts is not None:
                pts_int = pts.astype(int)
                n = len(pts_int[0])
                for i in range(n):
                    cv2.line(frame,
                             tuple(pts_int[0][i]),
                             tuple(pts_int[0][(i+1) % n]),
                             (0, 255, 0), 2)

            if qr_data.lower().startswith("ur:eth-sign-request/"):
                try:
                    tx      = decode_eip4527_ur(qr_data)
                    signing = _sign_eip4527(tx)
                    _emit({"event": "eip4527_detected", "transaction": tx, "signing": signing})
                except Exception as e:
                    _emit({"event": "eip4527_error", "error": str(e), "raw": qr_data[:120]})
            else:
                _emit({"event": "qr_detected", "data": qr_data[:256]})
        elif not qr_data:
            last_ur = None

        cv2.putText(frame, "EIP-4527 Signer  |  press q to quit",
                    (10, 28), cv2.FONT_HERSHEY_SIMPLEX, 0.65, (0, 200, 255), 2)
        cv2.imshow("eth_signer — Scan Transaction QR", frame)

        if cv2.waitKey(1) & 0xFF == ord('q'):
            break

    cap.release()
    cv2.destroyAllWindows()


# ── Output helpers ────────────────────────────────────────────────────────────

def _emit(obj: dict) -> None:
    print(json.dumps(obj, indent=2, default=str))
    sys.stdout.flush()


def _warn(obj: dict) -> None:
    print(json.dumps(obj, indent=2, default=str), file=sys.stderr)


def _die(msg: str) -> None:
    _warn({"error": msg})
    sys.exit(1)


# ── CLI ───────────────────────────────────────────────────────────────────────

def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="eth_signer.py",
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    # Modes
    p.add_argument("--generate-keys", action="store_true",
                   help="Generate a new ECDSA keypair (JSON output)")
    p.add_argument("--scan", action="store_true",
                   help="Open webcam and sign detected EIP-4527 QR codes")
    p.add_argument("--add-key", action="store_true",
                   help="Add a private key to the keys file")
    p.add_argument("--list-keys", action="store_true",
                   help="List stored addresses (private keys are never shown)")
    p.add_argument("--derive-address", metavar="PRIVATE_KEY",
                   help="Derive and print the address for a private key")

    # Signing params (used by --add-key and implicit sign mode)
    p.add_argument("--private-key", metavar="HEX",
                   help="Private key for signing or --add-key")
    p.add_argument("--to", metavar="ADDR",
                   help="Recipient Ethereum address")
    p.add_argument("--value", type=int, default=0,
                   help="Transfer value in wei (default: 0)")
    p.add_argument("--nonce", type=int, default=0,
                   help="Transaction nonce (default: 0)")
    p.add_argument("--gas-price", type=int, default=20_000_000_000,
                   help="Gas price in wei (default: 20 Gwei)")
    p.add_argument("--gas-limit", type=int, default=21_000,
                   help="Gas limit (default: 21000)")
    p.add_argument("--chain-id", type=int, default=1,
                   help="Chain ID (default: 1 = mainnet)")
    p.add_argument("--data", default="0x",
                   help="Calldata hex (default: 0x)")

    # --add-key name
    p.add_argument("--name", metavar="NAME",
                   help="Wallet name for --add-key (default: Unnamed)")

    return p


def main() -> None:
    parser = _build_parser()
    args   = parser.parse_args()

    try:
        if args.generate_keys:
            _emit(cmd_generate_keys())

        elif args.derive_address:
            addr = derive_address(args.derive_address)
            _emit({"private_key": args.derive_address, "public_key": addr})

        elif args.scan:
            cmd_scan()

        elif args.add_key:
            if not args.private_key:
                parser.error("--add-key requires --private-key")
            result = cmd_add_key(args.private_key, args.name or "Unnamed")
            _emit(result)

        elif args.list_keys:
            data    = load_keys()
            entries = [
                {"address": e["address"], "name": e.get("name", "")}
                for e in data.get("private_keys", [])
            ]
            _emit({"keys": entries, "file": str(KEYS_FILE)})

        elif args.private_key and args.to:
            result = cmd_sign(
                private_key=args.private_key,
                to=args.to,
                value=args.value,
                nonce=args.nonce,
                gas_price=args.gas_price,
                gas_limit=args.gas_limit,
                chain_id=args.chain_id,
                data_hex=args.data,
            )
            _emit(result)

        else:
            parser.print_help()
            sys.exit(1)

    except Exception as e:
        _die(str(e))


if __name__ == "__main__":
    main()
