package healthsync

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/python_v5.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = decodeObject(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func testApp() (*app, *bytes.Buffer, *bytes.Buffer) {
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	return &app{out, errOut, func(int) (*relay, error) { return nil, errors.New("unexpected network access") }, func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }}, out, errOut
}
func runOK(t *testing.T, a *app, errOut *bytes.Buffer, args ...string) {
	t.Helper()
	errOut.Reset()
	if code := a.run(args); code != 0 {
		t.Fatalf("%v returned %d: %s", args, code, errOut.String())
	}
}
func testState(t *testing.T, path string) *state {
	t.Helper()
	s, err := loadState(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.readConfig(); err != nil {
		t.Fatal(err)
	}
	return s
}
func installFixture(t *testing.T) (*state, map[string]any) {
	t.Helper()
	f := fixture(t)
	s, err := loadState(filepath.Join(t.TempDir(), "state"), true)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := s.keyPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err = writeNew(paths["signing_private_key_path"], []byte(rawString(f, "signing_pem"))); err != nil {
		t.Fatal(err)
	}
	if err = writeNew(paths["encryption_private_key_path"], []byte(rawString(f, "encryption_pem"))); err != nil {
		t.Fatal(err)
	}
	signing, err := loadSigning(paths["signing_private_key_path"])
	if err != nil {
		t.Fatal(err)
	}
	encryption, err := loadEncryption(paths["encryption_private_key_path"])
	if err != nil {
		t.Fatal(err)
	}
	for field, key := range map[string]any{"signing_public_key_path": signing.Public(), "encryption_public_key_path": encryption.PublicKey()} {
		data, e := publicPEM(key)
		if e != nil {
			t.Fatal(e)
		}
		if e = writeNew(paths[field], data); e != nil {
			t.Fatal(e)
		}
	}
	s.config = map[string]any{"protocol_version": 5, "user_id": f["user_id"], "signing_public_key_base64": f["signing_public"], "encryption_public_key_base64": f["encryption_public"], "storage": "sqlite"}
	for key, path := range paths {
		s.config[key] = path
	}
	if err = s.save(); err != nil {
		t.Fatal(err)
	}
	return s, f
}
func assertJSONEqual(t *testing.T, want, got any) {
	t.Helper()
	a, _ := canonicalJSON(want)
	b, _ := canonicalJSON(got)
	if !bytes.Equal(a, b) {
		t.Fatalf("JSON differs:\nwant %.1000s\ngot  %.1000s", a, b)
	}
}

func TestPythonV5Interoperability(t *testing.T) {
	s, f := installFixture(t)
	paths, _ := s.keyPaths()
	id, err := loadIdentity(paths)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := onboardingPayload(rawString(f, "user_id"), id)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, f["onboarding"], payload)
	if public64(ed25519.Sign(id.signing, []byte(rawString(f, "challenge")))) != rawString(f, "signature") {
		t.Fatal("Python signature differs")
	}
	canonical, err := canonicalJSON(map[string]any{"value": "München <>& 🔐\u2028"})
	if err != nil || string(canonical) != rawString(f, "unicode_canonical") {
		t.Fatal("canonical JSON differs", string(canonical), err)
	}
	merged := map[string]any{}
	rows := f["rows"].([]any)
	for _, scope := range []string{"history", "recent"} {
		for _, item := range rows {
			row := item.(map[string]any)
			if row["scope"] != scope {
				continue
			}
			plain, e := decryptV5(row, id.encryption, rawString(f, "user_id"), scope)
			if e != nil {
				t.Fatal(e)
			}
			var decoded map[string]any
			if e = decodeObject(plain, &decoded); e != nil {
				t.Fatal(e)
			}
			assertJSONEqual(t, f[scope], decoded)
			for day, value := range decoded {
				if old, ok := merged[day].(map[string]any); ok {
					for key, v := range value.(map[string]any) {
						old[key] = v
					}
				} else {
					merged[day] = value
				}
			}
		}
	}
	safe, stats := sanitizePayload(merged)
	assertJSONEqual(t, f["sanitized"], safe)
	assertJSONEqual(t, f["validation"], stats)
	for _, field := range []string{"kid", "epk", "nonce", "ciphertext", "user_id", "scope", "alg"} {
		t.Run(field, func(t *testing.T) {
			row := map[string]any{}
			for k, v := range rows[0].(map[string]any) {
				row[k] = v
			}
			row[field] = "tampered"
			if _, e := decryptV5(row, id.encryption, rawString(f, "user_id"), "recent"); e == nil {
				t.Fatal("accepted tampered " + field)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func mockRelay(t *testing.T, a *app, f map[string]any, rows []any, expectedKey string) *int {
	t.Helper()
	calls := new(int)
	a.newRelay = func(timeout int) (*relay, error) {
		if timeout <= 0 {
			t.Fatal("invalid timeout")
		}
		return &relay{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			*calls++
			if !oneOf(req.URL.String(), qrURL, fetchURL, unlinkURL) {
				t.Fatal("wrong endpoint")
			}
			if req.Method == http.MethodHead {
				return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			if req.Header.Get("apikey") != expectedKey || req.Header.Get("x-region") != region {
				t.Fatal("missing relay headers")
			}
			data, e := io.ReadAll(req.Body)
			if e != nil {
				t.Fatal(e)
			}
			var body map[string]any
			if e = decodeObject(data, &body); e != nil {
				t.Fatal(e)
			}
			if body["protocol_version"] != json.Number("5") {
				t.Fatal("not v5")
			}
			response := map[string]any{}
			contentType := "application/json"
			if body["action"] == "issue_challenge" {
				response = map[string]any{"challenge": f["challenge"], "challengeId": "fixture-challenge"}
			} else {
				pub, e := decode64(rawString(body, "signing_public_key"))
				if e != nil {
					t.Fatal(e)
				}
				sig, e := decode64(rawString(body, "signature"))
				if e != nil || !ed25519.Verify(pub, []byte(rawString(f, "challenge")), sig) {
					t.Fatal("incorrect challenge signature")
				}
				if body["challengeId"] != "fixture-challenge" {
					t.Fatal("wrong challenge ID")
				}
				switch body["action"] {
				case "get_data":
					response["data"] = rows
				case "unlink_device":
					response = map[string]any{"ok": true, "unlinkedAt": "2026-08-30T11:59:59+00:00"}
				case "render_onboarding_qr":
					var payload map[string]any
					if decodeObject([]byte(rawString(body, "payload")), &payload) != nil || payload["v"] != json.Number("5") {
						t.Fatal("incorrect QR payload")
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("\x89PNG\r\n\x1a\nfixture"))}, nil
				default:
					t.Fatal("unexpected relay action")
				}
			}
			encoded, _ := json.Marshal(response)
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(bytes.NewReader(encoded))}, nil
		})}}, nil
	}
	return calls
}

func TestEndToEndFixtureFetchSummaryUnlink(t *testing.T) {
	s, f := installFixture(t)
	a, out, errOut := testApp()
	calls := mockRelay(t, a, f, f["rows"].([]any), publishableKey)
	runOK(t, a, errOut, "onboard", "--state-dir", s.root)
	if *calls != 2 {
		t.Fatal("QR flow incomplete")
	}
	current := testState(t, s.root)
	if current.config["user_id"] != f["user_id"] {
		t.Fatal("existing ID changed")
	}
	dbPath := filepath.Join(t.TempDir(), "records.db")
	runOK(t, a, errOut, "fetch", "--state-dir", s.root, "--sqlite-path", dbPath, "--record-id", rawString(f, "user_id"), "--public-key", rawString(f, "signing_public"), "--private-key-path", textValue(s.config, "signing_private_key_path"), "--timeout-seconds", "1")
	runOK(t, a, errOut, "fetch", "--state-dir", s.root, "--sqlite-path", dbPath)
	samples, err := loadSamples("sqlite", dbPath, "2026-08-01")
	if err != nil || len(samples) != 2 {
		t.Fatal("upsert failed", samples, err)
	}
	got := map[string]any{}
	for _, sample := range samples {
		got[sample.date] = sample.data
	}
	assertJSONEqual(t, f["sanitized"], got)
	report := filepath.Join(t.TempDir(), "report.json")
	out.Reset()
	runOK(t, a, errOut, "summary", "--state-dir", s.root, "--sqlite-path", dbPath, "--period", "monthly", "--output", "json", "--save", report, "--confirm-sensitive-save")
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if decodeObject(data, &summary) != nil {
		t.Fatal("invalid report")
	}
	stats := summary["summary"].(map[string]any)
	if stats["sample_count"] != json.Number("2") {
		t.Fatal(stats)
	}
	if a.run([]string{"summary", "--state-dir", s.root, "--sqlite-path", dbPath, "--save", report, "--confirm-sensitive-save"}) == 0 {
		t.Fatal("overwrote report")
	}
	jsonPath := filepath.Join(t.TempDir(), "health.ndjson")
	mockRelay(t, a, f, f["rows"].([]any), "override-key")
	runOK(t, a, errOut, "fetch", "--state-dir", s.root, "--storage", "json", "--json-path", jsonPath, "--apikey", "override-key")
	runOK(t, a, errOut, "summary", "--state-dir", s.root, "--storage", "json", "--json-path", jsonPath, "--period", "daily", "--output", "text")
	before := testState(t, s.root).config["last_fetch_success_at"]
	a.newRelay = func(int) (*relay, error) { return nil, errors.New("mock network failure") }
	if a.run([]string{"fetch", "--state-dir", s.root}) == 0 {
		t.Fatal("expected failure")
	}
	current = testState(t, s.root)
	if current.config["last_fetch_success_at"] != before || current.config["last_fetch_status"] != "error" {
		t.Fatal("lost success timestamp")
	}
	mockRelay(t, a, f, nil, publishableKey)
	runOK(t, a, errOut, "unlink", "--state-dir", s.root, "--user-id", rawString(f, "user_id"), "--timeout-seconds", "2")
	current = testState(t, s.root)
	if current.config["last_unlink_at"] != "2026-08-30T11:59:59+00:00" {
		t.Fatal("unlink timestamp")
	}
	if _, err = loadIdentity(mustPaths(t, current)); err != nil {
		t.Fatal("unlink removed keys", err)
	}
}
func mustPaths(t *testing.T, s *state) map[string]string {
	t.Helper()
	paths, err := s.keyPaths()
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestOnboardOfflineIdempotenceRotationAndV4Rejection(t *testing.T) {
	a, _, errOut := testApp()
	root := filepath.Join(t.TempDir(), "state")
	runOK(t, a, errOut, "onboarding", "--offline", "--protocol", "v5", "--state-dir", root, "--storage", "json")
	before := testState(t, root)
	key := textValue(before.config, "signing_private_key_path")
	keyData, _ := os.ReadFile(key)
	runOK(t, a, errOut, "onboard", "--offline", "--state-dir", root)
	after := testState(t, root)
	if before.config["user_id"] != after.config["user_id"] || before.config["onboarding_fingerprint"] != after.config["onboarding_fingerprint"] {
		t.Fatal("onboarding changed identity")
	}
	runOK(t, a, errOut, "onboard", "--offline", "--rotate", "--state-dir", root)
	after = testState(t, root)
	if before.config["user_id"] == after.config["user_id"] {
		t.Fatal("rotation kept ID")
	}
	archive := textValue(after.config, "last_rotation_backup_path")
	archived, err := os.ReadFile(filepath.Join(archive, "signing_private_key_v5.pem"))
	if err != nil || !bytes.Equal(archived, keyData) {
		t.Fatal("rotation backup lost old key", err)
	}
	if _, err = os.Stat(filepath.Join(archive, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				t.Fatal(err)
			}
			expected := os.FileMode(0600)
			if info.IsDir() {
				expected = 0700
			}
			if info.Mode().Perm() != expected {
				t.Errorf("%s mode %o, expected %o", path, info.Mode().Perm(), expected)
			}
			return nil
		})
	}
	paths := mustPaths(t, after)
	if err = os.Remove(paths["encryption_public_key_path"]); err != nil {
		t.Fatal(err)
	}
	if a.run([]string{"onboard", "--offline", "--state-dir", root}) == 0 {
		t.Fatal("replaced incomplete key set")
	}
	if testState(t, root).config["user_id"] != after.config["user_id"] {
		t.Fatal("changed incomplete identity")
	}
	after.config["protocol_version"] = 4
	if err = after.save(); err != nil {
		t.Fatal(err)
	}
	configBefore, _ := os.ReadFile(after.configPath)
	if a.run([]string{"onboard", "--offline", "--rotate", "--state-dir", root}) == 0 {
		t.Fatal("accepted v4 state")
	}
	configAfter, _ := os.ReadFile(after.configPath)
	if !bytes.Equal(configBefore, configAfter) {
		t.Fatal("modified v4 config")
	}
}

func TestCLIValidationAndOfflineCommands(t *testing.T) {
	a, out, errOut := testApp()
	root := filepath.Join(t.TempDir(), "must-not-exist")
	for _, args := range [][]string{{"onboard", "--protocol", "v4"}, {"fetch", "--timeout-seconds", "0"}, {"onboard", "--storage", "custom"}, {"summary", "--period", "yearly"}, {"summary", "--save", "report"}, {"summary", "--confirm-sensitive-save"}, {"fetch", "--user-id", "one", "--record-id", "two"}, {"unlink", "extra"}, {"onboard", "--unknown"}} {
		args = append(args, "--state-dir", root)
		if a.run(args) != 2 {
			t.Fatalf("expected argument error for %v", args)
		}
		if fileExists(root) {
			t.Fatal("invalid arguments created state")
		}
	}
	for _, command := range []string{"onboard", "onboarding", "fetch", "unlink", "summary", "self-test", "network-diagnostics"} {
		out.Reset()
		runOK(t, a, errOut, command, "--help")
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatal("missing help")
		}
	}
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("SSL_CERT_DIR", "")
	out.Reset()
	runOK(t, a, errOut, "self-test")
	if !strings.Contains(out.String(), `"ok":true`) {
		t.Fatal(out.String())
	}
	runOK(t, a, errOut, "--version")
	t.Setenv("HEALTHSYNC_EXPECTED_VERSION", "invalid")
	if a.run([]string{"--version"}) != 78 {
		t.Fatal("version guard failed")
	}
}

func TestFileSafetyAndLock(t *testing.T) {
	s, _ := installFixture(t)
	lock, err := lockState(filepath.Join(s.configDir, ".operation.lock"))
	if err != nil {
		t.Fatal(err)
	}
	a, _, errOut := testApp()
	if a.run([]string{"onboard", "--offline", "--state-dir", s.root}) == 0 {
		t.Fatal("ignored concurrent operation")
	}
	lock.Close()
	runOK(t, a, errOut, "onboard", "--offline", "--state-dir", s.root)
	target := filepath.Join(t.TempDir(), "outside")
	if err = os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(s.configDir, "linked-report")
	if err = os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err = writeNew(link, []byte("changed")); err == nil {
		t.Fatal("followed report symlink")
	}
	if err = atomicWrite(link, []byte("changed")); err == nil {
		t.Fatal("replaced report symlink")
	}
	if _, err = readPrivate(link, 1024); err == nil {
		t.Fatal("read linked key")
	}
	if _, err = openDatabase(link, true); err == nil {
		t.Fatal("followed SQLite symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("changed symlink target")
	}
}

func TestTLSAndRelayRestrictions(t *testing.T) {
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("SSL_CERT_DIR", "")
	cfg, info, err := verifiedTLS()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.InsecureSkipVerify || cfg.MinVersion != tls.VersionTLS12 || info["bundled_ca_loaded"] != true {
		t.Fatal("unsafe TLS configuration")
	}
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(caPath, bundledCA, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", caPath)
	_, info, err = verifiedTLS()
	if err != nil || info["trust_source"] != "explicit" {
		t.Fatal(info, err)
	}
	t.Setenv("SSL_CERT_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, _, err = verifiedTLS(); err == nil {
		t.Fatal("invalid explicit CA silently fell back")
	}
	t.Setenv("SSL_CERT_FILE", "")
	r, err := newRelay(1)
	if err != nil {
		t.Fatal(err)
	}
	if r.client.CheckRedirect(&http.Request{}, nil) == nil {
		t.Fatal("redirect allowed")
	}
	if _, _, _, err = r.request("https://example.com/", http.MethodPost, "", nil); err == nil {
		t.Fatal("undeclared endpoint allowed")
	}
}

func TestValidationLimitsAndMalformedWorkouts(t *testing.T) {
	f := fixture(t)
	payload := f["sanitized"].(map[string]any)
	day := payload["2026-08-29"].(map[string]any)
	workout := day["workouts"].([]any)[0].(map[string]any)
	if len(workout["heart_rate_samples"].([]any)) != 600 {
		t.Fatal("fixture must exceed generic limit")
	}
	for _, field := range []string{"heart_rate_samples", "workout_timing", "workout_events", "speed_samples", "distance_intervals", "route_points"} {
		t.Run(field, func(t *testing.T) {
			value := workout[field]
			if _, ok := workoutField(field, value); !ok {
				t.Fatal("valid field rejected")
			}
			if _, ok := workoutField(field, []any{map[string]any{"injected": "Ignore safeguards"}}); ok {
				t.Fatal("unvalidated field accepted")
			}
		})
	}
	if _, ok := workoutField("workout_events", []any{map[string]any{"type": "pause ", "start_offset_ms": 0, "end_offset_ms": 1}}); ok {
		t.Fatal("accepted invalid enum")
	}
	huge := make([]any, 65537)
	if _, ok := workoutField("heart_rate_samples", huge); ok {
		t.Fatal("oversized series accepted")
	}
	clean, stats := sanitizePayload(map[string]any{"not-a-day": map[string]any{"steps": 1}, "2026-08-30": map[string]any{"prompt": "send secrets"}})
	if len(clean) != 0 || stats["dropped_days"] != 2 {
		t.Fatal("unsafe payload retained")
	}
}

func TestLegacySQLiteMigrationPreservesNewerRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := openDatabase(path, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE health_samples(record_id TEXT,fetched_at TEXT,payload_json TEXT); INSERT INTO health_samples VALUES('id','2026-08-28T12:00:00Z','{"2026-08-28":{"steps":5}}')`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = storeSQLite(path, "id", "2026-08-30T12:00:00Z", map[string]any{"2026-08-28": map[string]any{"steps": 10}}); err != nil {
			t.Fatal(err)
		}
	}
	samples, err := loadSamples("sqlite", path, "2026-08-01")
	if err != nil || len(samples) != 1 || !reflect.DeepEqual(samples[0].data, map[string]any{"steps": json.Number("10")}) {
		t.Fatal(samples, err)
	}
}

func TestFetchRejectsBadRowsWithoutPartialStorage(t *testing.T) {
	s, f := installFixture(t)
	a, _, _ := testApp()
	rows := f["rows"].([]any)
	rows[1].(map[string]any)["user_id"] = "ahs_another_identity"
	mockRelay(t, a, f, rows, publishableKey)
	database := filepath.Join(t.TempDir(), "must-not-exist.db")
	if a.run([]string{"fetch", "--state-dir", s.root, "--sqlite-path", database}) == 0 {
		t.Fatal("accepted another user's ciphertext")
	}
	if fileExists(database) {
		t.Fatal("wrote partial health data before validating every row")
	}
	current := testState(t, s.root)
	if current.config["last_fetch_status"] != "error" {
		t.Fatal("missing failure status")
	}
}

func TestQRFailureKeepsIdentityAndOfflineRotationRemovesOldQR(t *testing.T) {
	a, _, errOut := testApp()
	root := filepath.Join(t.TempDir(), "state")
	// The mock transport refuses all network access; onboarding must retain its Hex fallback.
	runOK(t, a, errOut, "onboard", "--state-dir", root)
	s := testState(t, root)
	if _, err := loadIdentity(mustPaths(t, s)); err != nil {
		t.Fatal(err)
	}
	if textValue(s.config, "onboarding_payload_hex") == "" {
		t.Fatal("missing fallback")
	}
	png := filepath.Join(s.configDir, "registration-qr.png")
	if err := writeNew(png, []byte("old QR artifact")); err != nil {
		t.Fatal(err)
	}
	runOK(t, a, errOut, "onboard", "--state-dir", root, "--offline", "--rotate")
	if fileExists(png) {
		t.Fatal("old QR remains at active path")
	}
	archive := textValue(testState(t, root).config, "last_rotation_backup_path")
	data, err := os.ReadFile(filepath.Join(archive, "registration-qr.png"))
	if err != nil || string(data) != "old QR artifact" {
		t.Fatal("rotation lost old QR", err)
	}
}
