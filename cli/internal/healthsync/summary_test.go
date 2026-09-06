package healthsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These output fixtures were captured from the previous Python CLI with a fixed
// clock and synthetic local records. They require no Python runtime during tests.
func TestSummaryPythonOutputCompatibility(t *testing.T) {
	var fixture struct {
		Now        string `json:"now"`
		SaveStdout string `json:"save_stdout"`
		SaveStderr string `json:"save_stderr"`
		Cases      []struct {
			Name    string `json:"name"`
			Period  string `json:"period"`
			Storage string `json:"storage"`
			Samples []struct {
				UserID  string         `json:"user_id"`
				Date    string         `json:"date"`
				Payload map[string]any `json:"payload"`
			} `json:"samples"`
			Text string `json:"text"`
			JSON string `json:"json"`
		} `json:"cases"`
	}
	data, err := os.ReadFile("testdata/python_summary.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339, fixture.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			root := t.TempDir()
			state, _ := installFixture(t)
			database := filepath.Join(root, "records.db")
			ndjson := filepath.Join(root, "records.ndjson")
			for _, sample := range tc.Samples {
				payload := map[string]any{sample.Date: sample.Payload}
				if tc.Storage == "sqlite" {
					err = storeSQLite(database, sample.UserID, fixture.Now, payload)
				} else {
					err = storeJSON(ndjson, map[string]any{"user_id": sample.UserID, "fetched_at": fixture.Now, "payload": payload})
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, format := range []string{"text", "json"} {
				t.Run(format, func(t *testing.T) {
					a, out, errOut := testApp()
					a.now = func() time.Time { return now }
					args := []string{"summary", "--state-dir", state.root, "--storage", tc.Storage,
						"--sqlite-path", database, "--json-path", ndjson, "--period", tc.Period, "--output", format}
					want := tc.Text
					if format == "json" {
						want = tc.JSON
					}
					runOK(t, a, errOut, args...)
					if out.String() != want {
						t.Errorf("output differs from Python:\nwant:\n%s\ngot:\n%s", want, out.String())
					}
					if errOut.Len() != 0 {
						t.Errorf("unexpected stderr: %s", errOut.String())
					}

					path := filepath.Join(root, "report."+format)
					saveArgs := append(append([]string{}, args...), "--save", path, "--confirm-sensitive-save")
					out.Reset()
					runOK(t, a, errOut, saveArgs...)
					if got, err := os.ReadFile(path); err != nil || string(got) != want {
						t.Errorf("saved report differs from Python: %v\n%s", err, got)
					}
					if want := strings.ReplaceAll(fixture.SaveStdout, "<REPORT_PATH>", path); out.String() != want {
						t.Errorf("save must print only the confirmation:\nwant: %q\ngot: %q", want, out.String())
					}
					if errOut.String() != fixture.SaveStderr {
						t.Errorf("save warning differs from Python: %q", errOut.String())
					}
					if runtime.GOOS != "windows" {
						info, err := os.Stat(path)
						if err != nil || info.Mode().Perm() != 0600 {
							t.Fatalf("saved report must remain private: %v", err)
						}
					}

					out.Reset()
					if a.run(saveArgs) == 0 {
						t.Error("existing report was overwritten")
					}
					if out.Len() != 0 {
						t.Errorf("failed save leaked report or success message: %s", out.String())
					}
				})
			}
		})
	}
}
