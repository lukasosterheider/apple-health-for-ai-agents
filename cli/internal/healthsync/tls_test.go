package healthsync

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Every dial is redirected to this in-process HTTPS server. No real relay request
// is possible, while Go's production TLS and redirect policy still run unchanged.
func TestVerifiedTLSHandshakeAndRedirects(t *testing.T) {
	endpoint, _ := url.Parse(qrURL)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic local test CA"}, DNSNames: []string{endpoint.Hostname()}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})
	path := filepath.Join(t.TempDir(), "synthetic-ca.pem")
	if err = os.WriteFile(path, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	var redirect atomic.Bool
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if redirect.Load() {
			w.Header().Set("Location", "https://example.com/not-allowed")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{certificate}, PrivateKey: private}}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	connect := func(r *relay) {
		transport := r.client.Transport.(*http.Transport)
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		}
	}
	t.Setenv("SSL_CERT_DIR", "")
	t.Setenv("SSL_CERT_FILE", path)
	r, err := newRelay(2)
	if err != nil {
		t.Fatal(err)
	}
	connect(r)
	if _, err = r.post(qrURL, "test-public-key", map[string]any{"action": "local-test"}); err != nil {
		t.Fatal("valid private CA rejected", err)
	}
	redirect.Store(true)
	if _, err = r.post(qrURL, "test-public-key", nil); err == nil {
		t.Fatal("followed relay redirect")
	}
	if calls.Load() != 2 {
		t.Fatal("sent a request after a redirect", calls.Load())
	}
	redirect.Store(false)
	t.Setenv("SSL_CERT_FILE", "")
	untrusted, err := newRelay(2)
	if err != nil {
		t.Fatal(err)
	}
	connect(untrusted)
	if _, err = untrusted.post(qrURL, "", nil); err == nil {
		t.Fatal("accepted untrusted TLS certificate")
	}
	t.Setenv("SSL_CERT_FILE", path)
	wrongHost, err := newRelay(2)
	if err != nil {
		t.Fatal(err)
	}
	connect(wrongHost)
	wrongHost.client.Transport.(*http.Transport).TLSClientConfig.ServerName = "incorrect.example"
	if _, err = wrongHost.post(qrURL, "", nil); err == nil {
		t.Fatal("accepted hostname mismatch")
	}
}
