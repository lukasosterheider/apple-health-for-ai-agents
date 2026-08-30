package healthsync

import (
	"bytes"
	"crypto/ed25519"
	"database/sql"
	"encoding/json"
	"errors"

	"golang.org/x/crypto/chacha20poly1305"
)

func (a *app) selfTest() error {
	id, err := generateIdentity()
	if err != nil {
		return err
	}
	message := []byte("healthsync offline self-test")
	if !ed25519.Verify(id.signing.Public().(ed25519.PublicKey), message, ed25519.Sign(id.signing, message)) {
		return errors.New("Ed25519 self-test failed")
	}
	peer, err := generateIdentity()
	if err != nil {
		return err
	}
	left, err := id.encryption.ECDH(peer.encryption.PublicKey())
	if err != nil {
		return err
	}
	right, err := peer.encryption.ECDH(id.encryption.PublicKey())
	if err != nil {
		return err
	}
	if !bytes.Equal(left, right) {
		return errors.New("X25519 self-test failed")
	}
	key, err := deriveKey(left, message)
	if err != nil {
		return err
	}
	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		return err
	}
	nonce := make([]byte, 12)
	ciphertext := cipher.Seal(nil, nonce, message, message)
	plain, err := cipher.Open(nil, nonce, ciphertext, message)
	if err != nil || !bytes.Equal(plain, message) {
		return errors.New("AEAD self-test failed")
	}
	_, tlsInfo, err := verifiedTLS()
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = db.Exec(schema); err != nil {
		return err
	}
	var n int
	if err = db.QueryRow("SELECT 42").Scan(&n); err != nil || n != 42 {
		return errors.New("SQLite self-test failed")
	}
	return json.NewEncoder(a.out).Encode(map[string]any{"ok": true, "runtime_version": Version, "protocol": 5, "default_storage": "sqlite", "cryptography": "ok", "sqlite": "ok", "tls": tlsInfo})
}
