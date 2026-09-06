package healthsync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDatabaseAbsoluteAndRelativePaths(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"health.db", "health data #100% ä.db"} {
		for _, absolute := range []bool{false, true} {
			path := filepath.Join("relative", name)
			if absolute {
				path = filepath.Join(t.TempDir(), name)
			}
			t.Run(path, func(t *testing.T) {
				db, err := openDatabase(path, true)
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec("CREATE TABLE check_path (value TEXT); INSERT INTO check_path VALUES ('retained')")
				db.Close()
				if err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(path); err != nil {
					t.Fatal(err)
				}
				db, err = openDatabase(path, false)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var value string
				if err = db.QueryRow("SELECT value FROM check_path").Scan(&value); err != nil {
					t.Fatal(err)
				}
				if value != "retained" {
					t.Fatalf("database did not retain the stored value: %q", value)
				}
			})
		}
	}
}

func TestLoadJSONSamplesSkipsMalformedLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.ndjson")
	rows := []map[string]any{
		{"user_id": "", "fetched_at": "2026-08-30T12:00:00Z", "payload": map[string]any{"2026-08-30": map[string]any{"steps": 1}}},
		{"user_id": "missing-fetched-at", "payload": map[string]any{"2026-08-30": map[string]any{"steps": 2}}},
		{"user_id": "bad-fetched-at", "fetched_at": "not-a-time", "payload": map[string]any{"2026-08-30": map[string]any{"steps": 3}}},
		{"user_id": "bad-day", "fetched_at": "2026-08-30T12:00:00Z", "payload": map[string]any{"2026-02-30": map[string]any{"steps": 4}}},
		{"user_id": "boundary", "fetched_at": "2026-08-30T12:00:00Z", "payload": map[string]any{"2026-08-23": map[string]any{"steps": 5}}},
		{"record_id": "legacy", "fetched_at": "2026-08-30T12:00:00Z", "payload": map[string]any{"2026-08-24": map[string]any{"steps": 6}}},
		{"user_id": "timestamped", "fetched_at": "2026-08-30T12:00:00", "payload": map[string]any{"2026-08-23T13:00:00Z": map[string]any{"steps": 7}}},
	}
	for _, row := range rows {
		if err := storeJSON(path, row); err != nil {
			t.Fatal(err)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString("not json\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	samples, err := loadSamples("json", path, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected two valid samples, got %#v", samples)
	}
	if samples[0].userID != "timestamped" || samples[0].date != "2026-08-23T13:00:00Z" {
		t.Fatalf("timestamp filtering or ordering changed: %#v", samples[0])
	}
	if samples[1].userID != "legacy" || samples[1].date != "2026-08-24" {
		t.Fatalf("record_id fallback changed: %#v", samples[1])
	}
}

func TestLoadSQLiteSamplesSkipsMalformedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.db")
	if err := ensureDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := openDatabase(path, false)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(map[string]any{"steps": 10})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	rows := [][]any{
		{"", "2026-08-30", string(valid), "2026-08-30T12:00:00Z"},
		{"bad-day", "2026-08-99", string(valid), "2026-08-30T12:00:00Z"},
		{"bad-update", "2026-08-29", string(valid), "not-a-time"},
		{"bad-data", "2026-08-29", "[]", "2026-08-30T12:00:00Z"},
		{"valid", "2026-08-28", string(valid), "2026-08-30T12:00:00Z"},
	}
	for _, row := range rows {
		if _, err = db.Exec("INSERT INTO health_data(user_id,date,data,created_at,updated_at) VALUES(?,?,?,?,?)", row[0], row[1], row[2], row[3], row[3]); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	samples, err := loadSamples("sqlite", path, time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].userID != "valid" || samples[0].date != "2026-08-28" {
		t.Fatalf("malformed SQLite rows affected the summary: %#v", samples)
	}
}
