package healthsync

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var keyNames = map[string]string{"signing_private_key_path": "signing_private_key_v5.pem", "signing_public_key_path": "signing_public_key_v5.pem", "encryption_private_key_path": "encryption_private_key_v5.pem", "encryption_public_key_path": "encryption_public_key_v5.pem"}

func (s *state) keyPaths() (map[string]string, error) {
	paths := map[string]string{}
	for field, name := range keyNames {
		dir := s.configDir
		if strings.Contains(field, "private") {
			dir = s.secretsDir
		}
		path, err := absolutePath(first(textValue(s.config, field), filepath.Join(dir, name)))
		if err != nil {
			return nil, err
		}
		paths[field] = path
	}
	return paths, nil
}
func loadIdentity(paths map[string]string) (*identity, error) {
	signing, err := loadSigning(paths["signing_private_key_path"])
	if err != nil {
		return nil, err
	}
	encryption, err := loadEncryption(paths["encryption_private_key_path"])
	if err != nil {
		return nil, err
	}
	if err = checkPublic(paths["signing_public_key_path"], signing.Public()); err != nil {
		return nil, err
	}
	if err = checkPublic(paths["encryption_public_key_path"], encryption.PublicKey()); err != nil {
		return nil, err
	}
	return &identity{signing, encryption}, nil
}
func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "ahs_" + base64.RawURLEncoding.EncodeToString(value), nil
}

// Every archive is complete and verified before the active config can change.
func (a *app) archiveIdentity(s *state, paths map[string]string, replacement string) (string, error) {
	if _, err := loadIdentity(paths); err != nil {
		return "", fmt.Errorf("cannot rotate an incomplete identity: %w", err)
	}
	root := filepath.Join(s.configDir, "key-backups")
	if err := secureDir(root); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(root, ".rotation-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	if err = restrictPath(staging, true); err != nil {
		return "", err
	}
	artifacts := map[string]string{"config.json": s.configPath}
	if _, err = os.Lstat(s.configPath); errors.Is(err, os.ErrNotExist) {
		delete(artifacts, "config.json")
	}
	for field, path := range paths {
		artifacts[keyNames[field]] = path
	}
	for _, name := range []string{"runtime.json", "registration-qr.json", "registration-qr.png", "public_key.pem"} {
		p := filepath.Join(s.configDir, name)
		if _, e := os.Lstat(p); e == nil {
			artifacts[name] = p
		} else if !errors.Is(e, os.ErrNotExist) {
			return "", e
		}
	}
	if p := filepath.Join(s.secretsDir, "private_key.pem"); fileExists(p) {
		artifacts["private_key.pem"] = p
	}
	var files []map[string]any
	for name, path := range artifacts {
		data, e := readPrivate(path, 64<<20)
		if e != nil {
			return "", e
		}
		target := filepath.Join(staging, name)
		if e = writeNew(target, data); e != nil {
			return "", e
		}
		copy, e := readPrivate(target, 64<<20)
		if e != nil || !bytes.Equal(data, copy) {
			return "", errors.New("rotation backup verification failed")
		}
		files = append(files, map[string]any{"path": name, "original_path": path, "sha256": shaHex(data)})
	}
	manifest := map[string]any{"archive_version": 1, "archived_at": a.timestamp(), "previous_user_id": textValue(s.config, "user_id"), "replacement_user_id": replacement, "previous_protocol_version": 5, "previous_onboarding_fingerprint": s.config["onboarding_fingerprint"], "files": files}
	if err = writeJSON(filepath.Join(staging, "manifest.json"), manifest); err != nil {
		return "", err
	}
	final := filepath.Join(root, a.now().UTC().Format("20060102T150405.000000000Z")+"-"+strings.TrimPrefix(replacement, "ahs_"))
	if err = os.Rename(staging, final); err != nil {
		return "", err
	}
	return final, nil
}
func fileExists(path string) bool { _, err := os.Lstat(path); return err == nil }

func (a *app) onboard(s *state, o options) error {
	oldPaths, err := s.keyPaths()
	if err != nil {
		return err
	}
	existing := len(s.config) > 0
	userID := textValue(s.config, "user_id")
	if existing && userID == "" {
		return errors.New("existing identity has no user ID; restore its configuration")
	}
	paths := oldPaths
	var id *identity
	var backup, staging string
	if existing && !o.rotate {
		id, err = loadIdentity(paths)
		if err != nil {
			return fmt.Errorf("existing v5 identity is incomplete; no keys were replaced: %w", err)
		}
	} else {
		userID, err = randomID()
		if err != nil {
			return err
		}
		if existing {
			backup, err = a.archiveIdentity(s, oldPaths, userID)
			if err != nil {
				return err
			}
		}
		if !existing {
			for _, path := range paths {
				if fileExists(path) {
					return errors.New("key files exist without configuration; restore the configuration or use a separate state directory")
				}
			}
		}
		id, err = generateIdentity()
		if err != nil {
			return err
		}
		// A new generation plus one atomic config switch avoids replacing keys in place.
		root := filepath.Join(s.secretsDir, "identities")
		if err = secureDir(root); err != nil {
			return err
		}
		staging = filepath.Join(root, userID)
		if err = os.Mkdir(staging, 0700); err != nil {
			return err
		}
		if err = restrictPath(staging, true); err != nil {
			return err
		}
		committed := false
		defer func() {
			if !committed {
				os.RemoveAll(staging)
			}
		}()
		paths = map[string]string{}
		keys := map[string]any{"signing_private_key_path": id.signing, "signing_public_key_path": id.signing.Public(), "encryption_private_key_path": id.encryption, "encryption_public_key_path": id.encryption.PublicKey()}
		for field, key := range keys {
			path := filepath.Join(staging, keyNames[field])
			var data []byte
			if strings.Contains(field, "private") {
				data, err = privatePEM(key)
			} else {
				data, err = publicPEM(key)
			}
			if err != nil {
				return err
			}
			if err = writeNew(path, data); err != nil {
				return err
			}
			paths[field] = path
		}
		// Once config points at these keys, later QR failures must not remove them.
		return a.finishOnboard(s, o, id, paths, userID, backup, func() { committed = true })
	}
	return a.finishOnboard(s, o, id, paths, userID, backup, func() {})
}

func (a *app) finishOnboard(s *state, o options, id *identity, paths map[string]string, userID, backup string, commit func()) error {
	kind, _, err := s.storage(o)
	if err != nil {
		return err
	}
	sqlitePath, err := absolutePath(first(textValue(s.config, "sqlite_path"), filepath.Join(s.root, "health_data.db")))
	if err != nil {
		return err
	}
	jsonPath, err := absolutePath(first(textValue(s.config, "json_path"), filepath.Join(s.configDir, "health_data.ndjson")))
	if err != nil {
		return err
	}
	if err = ensureDatabase(sqlitePath); err != nil {
		return err
	}
	payload, err := onboardingPayload(userID, id)
	if err != nil {
		return err
	}
	compact, err := canonicalJSON(payload)
	if err != nil {
		return err
	}
	config := map[string]any{}
	for key, value := range s.config {
		if o.rotate && (strings.HasPrefix(key, "last_fetch_") || strings.HasPrefix(key, "last_unlink_") || strings.HasPrefix(key, "last_validation_")) {
			continue
		}
		config[key] = value
	}
	for _, key := range []string{"private_key_path", "public_key_path", "public_key_base64", "qr_svg_path"} {
		delete(config, key)
	}
	values := map[string]any{"user_id": userID, "protocol_version": 5, "algorithm": "Ed25519", "signing_algorithm": "Ed25519", "encryption_algorithm": "X25519", "box_algorithm": boxAlgorithm, "state_dir": s.root, "config_dir": s.configDir, "secrets_dir": s.secretsDir, "signing_public_key_base64": public64(id.signing.Public().(ed25519.PublicKey)), "encryption_public_key_base64": public64(id.encryption.PublicKey().Bytes()), "onboarding_fingerprint": payload["fingerprint"], "onboarding_payload_json": string(compact), "onboarding_payload_hex": hex.EncodeToString(compact), "storage": kind, "sqlite_path": sqlitePath, "json_path": jsonPath, "qr_payload_path": filepath.Join(s.configDir, "registration-qr.json"), "qr_png_path": "", "updated_at": a.timestamp()}
	for key, value := range values {
		config[key] = value
	}
	for key, value := range paths {
		config[key] = value
	}
	if o.rotate {
		config["last_rotation_at"] = a.timestamp()
		config["last_rotation_backup_path"] = backup
	}
	if err = writeJSON(s.configPath, config); err != nil {
		return err
	}
	commit()
	s.config = config
	if o.rotate {
		// The previous QR was archived. Never leave its image at the active path.
		if err = os.Remove(filepath.Join(s.configDir, "registration-qr.png")); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err = writeJSON(textValue(config, "qr_payload_path"), payload); err != nil {
		return err
	}
	if o.offline {
		fmt.Fprintln(a.out, "Offline onboarding: no relay requests were made.")
	} else {
		pngPath, qrErr := a.renderQR(s, id, string(compact), userID)
		if qrErr != nil {
			fmt.Fprintf(a.errOut, "QR unavailable: %v. The local identity and Hex payload are ready.\n", qrErr)
		} else {
			s.config["qr_png_path"] = pngPath
			if err = s.save(); err != nil {
				return err
			}
			fmt.Fprintln(a.out, "QR PNG: "+pngPath)
		}
	}
	fmt.Fprintf(a.out, "Initialization complete.\nState dir: %s\nUser ID: %s\nProtocol: v5\nConfig: %s\n", s.root, userID, s.configPath)
	if backup != "" {
		fmt.Fprintln(a.out, "Previous identity backup: "+backup)
		fmt.Fprintln(a.out, "Rotation created a new user ID. Existing server data remains associated with the archived identity.")
	}
	if textValue(s.config, "qr_png_path") == "" {
		fmt.Fprintln(a.out, "Hex onboarding payload: config.json field onboarding_payload_hex")
	}
	fmt.Fprintln(a.out, "iOS app link: "+appStoreURL)
	return nil
}
func (a *app) renderQR(s *state, id *identity, payload, userID string) (string, error) {
	r, err := a.newRelay(10)
	if err != nil {
		return "", err
	}
	body, err := r.signedRequest(qrURL, publishableKey, "render_onboarding_qr", userID, id.signing)
	if err != nil {
		return "", err
	}
	body["payload"] = payload
	data, contentType, _, err := r.request(qrURL, http.MethodPost, publishableKey, body)
	if err != nil {
		return "", err
	}
	if !strings.Contains(strings.ToLower(contentType), "image/png") || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) || len(data) > 4<<20 {
		return "", errors.New("relay returned an invalid QR PNG")
	}
	path := filepath.Join(s.configDir, "registration-qr.png")
	return path, atomicWrite(path, data)
}
