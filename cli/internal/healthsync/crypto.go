package healthsync

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"

	"golang.org/x/crypto/chacha20poly1305"
)

const boxAlgorithm = "X25519-ChaCha20Poly1305"

type identity struct {
	signing    ed25519.PrivateKey
	encryption *ecdh.PrivateKey
}

func generateIdentity() (*identity, error) {
	_, signing, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	encryption, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &identity{signing, encryption}, nil
}
func public64(key []byte) string { return base64.StdEncoding.EncodeToString(key) }
func decode64(value string) ([]byte, error) {
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
	if len(value)%4 != 0 {
		value += strings.Repeat("=", 4-len(value)%4)
	}
	return base64.StdEncoding.Strict().DecodeString(value)
}
func readPEM(path string) (*pem.Block, error) {
	data, err := readPrivate(path, 64<<10)
	if err != nil {
		return nil, err
	}
	block, rest := pem.Decode(data)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("invalid PEM key file")
	}
	return block, nil
}
func loadSigning(path string) (ed25519.PrivateKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid signing private key")
	}
	value, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("expected an Ed25519 private key; only v5 is supported")
	}
	return value, nil
}
func loadEncryption(path string) (*ecdh.PrivateKey, error) {
	block, err := readPEM(path)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid encryption private key")
	}
	value, ok := key.(*ecdh.PrivateKey)
	if !ok || value.Curve() != ecdh.X25519() {
		return nil, errors.New("expected an X25519 private key")
	}
	return value, nil
}
func checkPublic(path string, expected any) error {
	block, err := readPEM(path)
	if err != nil {
		return err
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return errors.New("invalid public key PEM")
	}
	actualDER, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return err
	}
	expectedDER, err := x509.MarshalPKIXPublicKey(expected)
	if err != nil {
		return err
	}
	if !bytes.Equal(actualDER, expectedDER) {
		return errors.New("public key does not match the private key; restore the complete identity")
	}
	return nil
}
func privatePEM(key any) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
func publicPEM(key any) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

// canonicalJSON matches Python's compact, sorted-key, ensure_ascii encoding used
// by the existing v5 protocol. It is used for fingerprints and authenticated AAD.
func canonicalJSON(value any) ([]byte, error) {
	var raw bytes.Buffer
	encoder := json.NewEncoder(&raw)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	var output strings.Builder
	for _, r := range strings.TrimSuffix(raw.String(), "\n") {
		if r <= 127 {
			output.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&output, "\\u%04x", r)
		} else {
			a, b := utf16.EncodeRune(r)
			fmt.Fprintf(&output, "\\u%04x\\u%04x", a, b)
		}
	}
	return []byte(output.String()), nil
}
func shaHex(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func onboardingPayload(userID string, id *identity) (map[string]any, error) {
	payload := map[string]any{"v": 5, "id": userID, "sig": map[string]any{"alg": "Ed25519", "publicKeyBase64": public64(id.signing.Public().(ed25519.PublicKey))}, "enc": map[string]any{"alg": "X25519", "box": boxAlgorithm, "publicKeyBase64": public64(id.encryption.PublicKey().Bytes())}}
	canonical, err := canonicalJSON(payload)
	if err != nil {
		return nil, err
	}
	payload["fingerprint"] = shaHex(canonical)
	return payload, nil
}
func keyID(public []byte) string {
	hash := sha256.Sum256(public)
	return base64.RawURLEncoding.EncodeToString(hash[:])
}
func deriveKey(shared, aad []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, shared, []byte("healthsync-v5"), string(aad), 32)
}
func decryptV5(row map[string]any, private *ecdh.PrivateKey, userID, scope string) ([]byte, error) {
	if !oneOf(scope, "history", "recent") || rawString(row, "scope") != scope || rawString(row, "user_id") != userID {
		return nil, errors.New("v5 row scope or user ID does not match the requested identity")
	}
	if rawString(row, "alg") != boxAlgorithm {
		return nil, errors.New("unsupported encrypted payload algorithm; only v5 is supported")
	}
	if rawString(row, "kid") != keyID(private.PublicKey().Bytes()) {
		return nil, errors.New("v5 payload was not addressed to this encryption key")
	}
	limit := 4 << 20
	if scope == "history" {
		limit = 16 << 20
	}
	if len(rawString(row, "ciphertext")) > base64.StdEncoding.EncodedLen(limit)+16 {
		return nil, errors.New("v5 ciphertext exceeds its size limit")
	}
	ephemeral, err := decode64(rawString(row, "epk"))
	if err != nil || len(ephemeral) != 32 {
		return nil, errors.New("invalid ephemeral X25519 public key")
	}
	nonce, err := decode64(rawString(row, "nonce"))
	if err != nil || len(nonce) != 12 {
		return nil, errors.New("invalid ChaCha20-Poly1305 nonce")
	}
	ciphertext, err := decode64(rawString(row, "ciphertext"))
	if err != nil || len(ciphertext) > limit {
		return nil, errors.New("invalid or oversized v5 ciphertext")
	}
	peer, err := ecdh.X25519().NewPublicKey(ephemeral)
	if err != nil {
		return nil, err
	}
	shared, err := private.ECDH(peer)
	if err != nil {
		return nil, errors.New("invalid X25519 key agreement")
	}
	aad, err := canonicalJSON(map[string]any{"scope": scope, "user_id": userID})
	if err != nil {
		return nil, err
	}
	key, err := deriveKey(shared, aad)
	if err != nil {
		return nil, err
	}
	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}
	plaintext, err := cipher.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("v5 payload authentication/decryption failed")
	}
	if len(plaintext) > limit {
		return nil, errors.New("v5 plaintext exceeds its size limit")
	}
	return plaintext, nil
}
