package healthsync

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
)

func signingRuntime(s *state, o options) (string, ed25519.PrivateKey, error) {
	userID := first(o.userID, o.recordID, textValue(s.config, "user_id"))
	if userID == "" {
		return "", nil, errors.New("missing user ID")
	}
	paths, err := s.keyPaths()
	if err != nil {
		return "", nil, err
	}
	path, err := absolutePath(first(o.privateKey, paths["signing_private_key_path"]))
	if err != nil {
		return "", nil, err
	}
	signing, err := loadSigning(path)
	if err != nil {
		return "", nil, err
	}
	expected := o.publicKey
	if expected == "" && o.privateKey == "" {
		expected = textValue(s.config, "signing_public_key_base64")
	}
	if expected != "" {
		pub, e := decode64(expected)
		if e != nil || !bytes.Equal(pub, signing.Public().(ed25519.PublicKey)) {
			return "", nil, errors.New("signing public key does not match private key")
		}
	}
	return userID, signing, nil
}
func (a *app) recordOperation(s *state, name, attempt string, opErr error) error {
	prefix := "last_" + name + "_"
	if textValue(s.config, prefix+"success_at") == "" && textValue(s.config, prefix+"at") != "" {
		if s.config[prefix+"status"] == "ok" {
			s.config[prefix+"success_at"] = s.config[prefix+"at"]
		} else {
			delete(s.config, prefix+"at")
		}
	}
	s.config[prefix+"attempt_at"] = attempt
	if opErr != nil {
		s.config[prefix+"status"] = "error"
		s.config[prefix+"error"] = opErr.Error()
	} else {
		s.config[prefix+"status"] = "ok"
		delete(s.config, prefix+"error")
		s.config[prefix+"success_at"] = a.timestamp()
		s.config[prefix+"at"] = s.config[prefix+"success_at"]
	}
	return errors.Join(opErr, s.save())
}
func (a *app) fetch(s *state, o options) (err error) {
	attempt := a.timestamp()
	defer func() { err = a.recordOperation(s, "fetch", attempt, err) }()
	userID, signing, err := signingRuntime(s, o)
	if err != nil {
		return err
	}
	paths, err := s.keyPaths()
	if err != nil {
		return err
	}
	encryption, err := loadEncryption(paths["encryption_private_key_path"])
	if err != nil {
		return err
	}
	if expected := textValue(s.config, "encryption_public_key_base64"); expected != "" {
		pub, e := decode64(expected)
		if e != nil || !bytes.Equal(pub, encryption.PublicKey().Bytes()) {
			return errors.New("configured encryption public key does not match private key")
		}
	}
	kind, path, err := s.storage(o)
	if err != nil {
		return err
	}
	r, err := a.newRelay(o.timeout)
	if err != nil {
		return err
	}
	key := first(o.apiKey, publishableKey)
	request, err := r.signedRequest(fetchURL, key, "get_data", userID, signing)
	if err != nil {
		return err
	}
	response, err := r.post(fetchURL, key, request)
	if err != nil {
		return err
	}
	rows, ok := response["data"].([]any)
	if !ok {
		return errors.New("relay response must contain a data array")
	}
	merged := map[string]any{}
	// Scope precedence is independent of the relay's row ordering.
	scopes := map[string][]map[string]any{"history": {}, "recent": {}}
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			return errors.New("invalid encrypted row")
		}
		scope := rawString(row, "scope")
		if !oneOf(scope, "history", "recent") || rawString(row, "alg") != boxAlgorithm {
			return errors.New("unsupported encrypted row; this CLI accepts only v5 history/recent data")
		}
		scopes[scope] = append(scopes[scope], row)
	}
	for _, scope := range []string{"history", "recent"} {
		for _, row := range scopes[scope] {
			plain, e := decryptV5(row, encryption, userID, scope)
			if e != nil {
				return e
			}
			var payload map[string]any
			if decodeObject(plain, &payload) != nil {
				return errors.New("decrypted health payload is not a JSON object")
			}
			for day, newValue := range payload {
				oldMap, oldOK := merged[day].(map[string]any)
				newMap, newOK := newValue.(map[string]any)
				if oldOK && newOK {
					for key, value := range newMap {
						oldMap[key] = value
					}
				} else {
					merged[day] = newValue
				}
			}
		}
	}
	safe, validation := sanitizePayload(merged)
	if len(merged) > 0 && len(safe) == 0 {
		return errors.New("no valid health days remained after validation; nothing was stored")
	}
	at := a.timestamp()
	if kind == "sqlite" {
		err = storeSQLite(path, userID, at, safe)
	} else {
		err = storeJSON(path, map[string]any{"user_id": userID, "fetched_at": at, "payload": safe, "row_count": len(rows), "validation": validation})
	}
	if err != nil {
		return err
	}
	s.config["last_fetch_row_count"] = len(rows)
	for key, value := range validation {
		s.config["last_validation_"+key] = value
	}
	fmt.Fprintf(a.out, "Fetched %d encrypted rows; stored %d days in %s: %s\n", len(rows), len(safe), kind, path)
	return nil
}
func (a *app) unlink(s *state, o options) (err error) {
	attempt := a.timestamp()
	var unlinkedAt string
	defer func() {
		err = a.recordOperation(s, "unlink", attempt, err)
		if err == nil && unlinkedAt != "" {
			s.config["last_unlink_success_at"] = unlinkedAt
			s.config["last_unlink_at"] = unlinkedAt
			err = s.save()
		}
	}()
	userID, signing, err := signingRuntime(s, o)
	if err != nil {
		return err
	}
	r, err := a.newRelay(o.timeout)
	if err != nil {
		return err
	}
	request, err := r.signedRequest(unlinkURL, publishableKey, "unlink_device", userID, signing)
	if err != nil {
		return err
	}
	result, err := r.post(unlinkURL, publishableKey, request)
	if err != nil {
		return err
	}
	if result["ok"] != true {
		return errors.New("relay did not confirm unlink")
	}
	unlinkedAt = textValue(result, "unlinkedAt")
	if strings.ContainsAny(unlinkedAt, "\r\n") {
		unlinkedAt = ""
	}
	fmt.Fprintln(a.out, "Device unlinked. Local keys and stored health data were kept.")
	return nil
}
