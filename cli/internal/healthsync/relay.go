package healthsync

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed resources/cacert.pem
var bundledCA []byte

//go:embed resources/THIRD_PARTY_NOTICES.txt
var thirdPartyNotices string

type relay struct {
	client      *http.Client
	diagnostics map[string]any
}

func verifiedTLS() (*tls.Config, map[string]any, error) {
	file, dir := os.Getenv("SSL_CERT_FILE"), os.Getenv("SSL_CERT_DIR")
	explicit := strings.TrimSpace(file) != "" || strings.TrimSpace(dir) != ""
	var roots *x509.CertPool
	source := "native+bundled"
	if explicit {
		roots = x509.NewCertPool()
		source = "explicit"
		loaded := false
		if file != "" {
			path, err := absolutePath(file)
			if err != nil {
				return nil, nil, err
			}
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				return nil, nil, errors.New("SSL_CERT_FILE must be a readable certificate file")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, nil, err
			}
			loaded = roots.AppendCertsFromPEM(data)
			if !loaded {
				return nil, nil, errors.New("SSL_CERT_FILE contains no certificates")
			}
		}
		if dir != "" {
			path, err := absolutePath(dir)
			if err != nil {
				return nil, nil, err
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return nil, nil, errors.New("SSL_CERT_DIR must be a readable certificate directory")
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					data, e := os.ReadFile(filepath.Join(path, entry.Name()))
					if e == nil && roots.AppendCertsFromPEM(data) {
						loaded = true
					}
				}
			}
		}
		if !loaded {
			return nil, nil, errors.New("explicit TLS trust contains no certificates")
		}
	} else {
		var err error
		roots, err = x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
			source = "bundled"
		}
		if !roots.AppendCertsFromPEM(bundledCA) {
			return nil, nil, errors.New("embedded CA certificates are invalid")
		}
	}
	diagnostics := map[string]any{"verification": "required", "hostname_verification": true, "minimum_tls": "1.2", "trust_source": source, "bundled_ca_loaded": !explicit, "explicit_ca_file": file != "", "explicit_ca_dir": dir != ""}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, diagnostics, nil
}
func newRelay(timeout int) (*relay, error) {
	config, diagnostics, err := verifiedTLS()
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = config
	client := &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("relay redirects are not allowed") }}
	return &relay{client, diagnostics}, nil
}
func (r *relay) request(url, method, apiKey string, payload any) ([]byte, string, int, error) {
	if !oneOf(url, qrURL, fetchURL, unlinkURL) {
		return nil, "", 0, errors.New("refusing undeclared relay URL")
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, "", 0, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, "", 0, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("apikey", apiKey)
		req.Header.Set("x-region", region)
	}
	response, err := r.client.Do(req)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, "", 0, fmt.Errorf("verified relay request failed: %w", err)
	}
	defer response.Body.Close()
	if method == http.MethodHead {
		return nil, response.Header.Get("Content-Type"), response.StatusCode, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", response.StatusCode, fmt.Errorf("relay returned HTTP %d", response.StatusCode)
	}
	const limit = 64 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", 0, err
	}
	if len(data) > limit {
		return nil, "", 0, errors.New("relay response exceeds size limit")
	}
	return data, response.Header.Get("Content-Type"), response.StatusCode, nil
}
func (r *relay) post(url, key string, payload any) (map[string]any, error) {
	data, _, _, err := r.request(url, http.MethodPost, key, payload)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err = decodeObject(data, &result); err != nil {
		return nil, errors.New("relay returned an invalid JSON object")
	}
	return result, nil
}
func (r *relay) signedRequest(url, key, action, userID string, signing ed25519.PrivateKey) (map[string]any, error) {
	challenge, err := r.post(url, key, map[string]any{"action": "issue_challenge", "id": userID, "protocol_version": 5})
	if err != nil {
		return nil, err
	}
	text, id := rawString(challenge, "challenge"), rawString(challenge, "challengeId")
	if text == "" || id == "" || len(text) > 65536 || len(id) > 4096 {
		return nil, errors.New("relay returned an invalid challenge")
	}
	return map[string]any{"action": action, "id": userID, "protocol_version": 5, "challengeId": id, "signature": public64(ed25519.Sign(signing, []byte(text))), "signing_public_key": public64(signing.Public().(ed25519.PublicKey))}, nil
}
func (a *app) networkDiagnostics(o options) error {
	r, err := a.newRelay(o.timeout)
	var status int
	if err == nil {
		_, _, status, err = r.request(qrURL, http.MethodHead, "", nil)
	}
	payload := map[string]any{"ok": err == nil, "endpoint": qrURL, "method": "HEAD"}
	if err != nil {
		payload["error"] = err.Error()
	} else {
		payload["http_status"] = status
		payload["tls"] = r.diagnostics
	}
	if printErr := json.NewEncoder(a.out).Encode(payload); printErr != nil {
		return printErr
	}
	return err
}
