package rpc

import (
	"hash/crc32"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// buildEthSignatureUR hand-encodes a CBOR eth-signature map (key 1: tag(37)
// 16-byte request-id, key 2: 65-byte signature) and bytewords-encodes it
// into a "ur:eth-signature/..." string, mirroring how a real EIP-4527
// offline signer would reply. It is the test-only inverse of
// buildEthSignRequestCBOR.
func buildEthSignatureUR(requestID [16]byte, signature [65]byte) string {
	var buf []byte
	buf = append(buf, 0xA2) // map(2)

	buf = append(buf, 0x01)       // key 1
	buf = append(buf, 0xD8, 0x25) // tag(37)
	buf = append(buf, 0x50)       // bytes(16)
	buf = append(buf, requestID[:]...)

	buf = append(buf, 0x02) // key 2
	buf = append(buf, cborBytesField(signature[:])...)

	checksum := crc32.ChecksumIEEE(buf)
	payload := append(buf, byte(checksum>>24), byte(checksum>>16), byte(checksum>>8), byte(checksum))
	return "ur:eth-signature/" + encodeBytewordsMinimal(payload)
}

func TestEth4527SignatureRoundTrip(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate throwaway key: %v", err)
	}
	from := crypto.PubkeyToAddress(privKey.PublicKey)
	to := common.HexToAddress("0x000000000000000000000000000000000000bEEF")
	value := big.NewInt(1_000_000_000_000_000) // 0.001 ETH
	nonce := uint64(7)
	tip := big.NewInt(1_000_000_000)
	maxFee := big.NewInt(20_000_000_000)
	gasLimit := uint64(21000)
	chainID := big.NewInt(1)

	urStr, txJSON, err := BuildUnsignedTxEIP4527(from, to, value, gasLimit, nil, nonce, tip, maxFee, chainID)
	if err != nil {
		t.Fatalf("BuildUnsignedTxEIP4527: %v", err)
	}
	if urStr == "" {
		t.Fatal("expected non-empty UR string")
	}

	parsedTo, parsedValue, parsedNonce, parsedTip, parsedMaxFee, parsedGasLimit, parsedChainID, parsedData, requestID, err := ParsePackagedTxJSON(txJSON)
	if err != nil {
		t.Fatalf("ParsePackagedTxJSON: %v", err)
	}
	if parsedTo != to || parsedNonce != nonce || parsedGasLimit != gasLimit {
		t.Fatalf("parsed fields mismatch: to=%v nonce=%d gasLimit=%d", parsedTo, parsedNonce, parsedGasLimit)
	}
	if parsedValue.Cmp(value) != 0 || parsedTip.Cmp(tip) != 0 || parsedMaxFee.Cmp(maxFee) != 0 || parsedChainID.Cmp(chainID) != 0 {
		t.Fatalf("parsed numeric fields mismatch")
	}
	if len(parsedData) != 0 {
		t.Fatalf("expected empty data, got %x", parsedData)
	}

	// Sign the same EIP-1559 signing hash go-ethereum's signer would compute,
	// exactly as a real offline signer would.
	unsignedTx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   parsedChainID,
		Nonce:     parsedNonce,
		GasTipCap: parsedTip,
		GasFeeCap: parsedMaxFee,
		Gas:       parsedGasLimit,
		To:        &parsedTo,
		Value:     parsedValue,
		Data:      parsedData,
	})
	signer := types.LatestSignerForChainID(parsedChainID)
	sigHash := signer.Hash(unsignedTx)
	sig, err := crypto.Sign(sigHash[:], privKey)
	if err != nil {
		t.Fatalf("crypto.Sign: %v", err)
	}
	var signature [65]byte
	copy(signature[:], sig)

	sigUR := buildEthSignatureUR(requestID, signature)

	frame, err := DecodeURFrame(sigUR)
	if err != nil {
		t.Fatalf("DecodeURFrame: %v", err)
	}
	if frame.Type != "eth-signature" {
		t.Fatalf("expected type eth-signature, got %q", frame.Type)
	}
	reassembler := NewURReassembler(frame)
	cborData, complete, err := reassembler.AddFrame(frame)
	if err != nil || !complete {
		t.Fatalf("AddFrame: complete=%v err=%v", complete, err)
	}

	decodedReqID, decodedSig, err := DecodeEthSignature(cborData)
	if err != nil {
		t.Fatalf("DecodeEthSignature: %v", err)
	}
	if decodedReqID != requestID {
		t.Fatalf("request-id mismatch: got %x want %x", decodedReqID, requestID)
	}
	if decodedSig != signature {
		t.Fatalf("signature mismatch")
	}

	rawHex, err := AssembleSignedTx(parsedChainID, parsedNonce, parsedTip, parsedMaxFee, parsedGasLimit, parsedTo, parsedValue, parsedData, decodedSig)
	if err != nil {
		t.Fatalf("AssembleSignedTx: %v", err)
	}

	decoded, err := DecodeSignedRawTx(rawHex)
	if err != nil {
		t.Fatalf("DecodeSignedRawTx: %v", err)
	}
	if decoded.From != from.Hex() {
		t.Fatalf("recovered sender mismatch: got %s want %s", decoded.From, from.Hex())
	}
	if decoded.To != to.Hex() {
		t.Fatalf("recovered recipient mismatch: got %s want %s", decoded.To, to.Hex())
	}
	if decoded.Nonce != nonce {
		t.Fatalf("recovered nonce mismatch: got %d want %d", decoded.Nonce, nonce)
	}
}

func TestEth4527MalformedInput(t *testing.T) {
	t.Run("not a UR string", func(t *testing.T) {
		if _, err := DecodeURFrame("not-a-ur-string"); err == nil {
			t.Fatal("expected error for non-UR string")
		}
	})

	t.Run("truncated bytewords", func(t *testing.T) {
		if _, err := DecodeURFrame("ur:eth-signature/abcde"); err == nil {
			t.Fatal("expected error for odd-length bytewords")
		}
	})

	t.Run("bad CRC", func(t *testing.T) {
		var reqID [16]byte
		var sig [65]byte
		good := buildEthSignatureUR(reqID, sig)
		// Flip the last bytewords character to corrupt the trailing CRC32.
		corrupted := good[:len(good)-2] + "zz"
		if _, err := DecodeURFrame(corrupted); err == nil {
			t.Fatal("expected CRC mismatch error")
		}
	})

	t.Run("wrong-size signature field", func(t *testing.T) {
		var buf []byte
		buf = append(buf, 0xA2)
		buf = append(buf, 0x01, 0xD8, 0x25, 0x50)
		buf = append(buf, make([]byte, 16)...)
		buf = append(buf, 0x02)
		buf = append(buf, cborBytesField(make([]byte, 10))...) // wrong size, should be 65
		if _, _, err := DecodeEthSignature(buf); err == nil {
			t.Fatal("expected error for wrong-size signature field")
		}
	})

	t.Run("ParsePackagedTxJSON missing requestId", func(t *testing.T) {
		if _, _, _, _, _, _, _, _, _, err := ParsePackagedTxJSON(`{"to":"0x0","value":"0x0","nonce":"0x0","maxPriorityFeePerGas":"0x0","maxFeePerGas":"0x0","gasLimit":"0x0","chainId":"0x1"}`); err == nil {
			t.Fatal("expected error for missing requestId")
		}
	})
}
