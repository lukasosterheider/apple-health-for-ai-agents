package healthsync

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `CREATE TABLE IF NOT EXISTS health_data (
 id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL, date TEXT NOT NULL,
 data TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS health_data_user_date_idx ON health_data(user_id,date);`

// An explicitly selected output directory belongs to the user. Do not chmod it.
func outputParent(path string, create bool) error {
	parent := filepath.Dir(path)
	if create {
		if err := os.MkdirAll(parent, 0700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("output parent must be a directory, not a symbolic link")
	}
	return nil
}
func openDatabase(path string, create bool) (*sql.DB, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := outputParent(path, create); err != nil {
		return nil, err
	}
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		info, err := os.Lstat(path + suffix)
		if err == nil {
			if !info.Mode().IsRegular() {
				return nil, errors.New("database or journal must not be a link or non-regular file")
			}
			if err = restrictPath(path+suffix, false); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	file, err := openPrivate(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	file.Close()
	if err = restrictPath(path, false); err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(path)
	// SQLite expects /C:/... for Windows drive paths, with no URI authority.
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	query := uri.Query()
	query.Set("mode", "rw")
	query.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func ensureDatabase(path string) error {
	db, err := openDatabase(path, true)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(schema)
	return err
}

func storeSQLite(path, userID, at string, payload map[string]any) error {
	db, err := openDatabase(path, true)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(schema); err != nil {
		return err
	}
	// Migrate the older local table without overwriting newer per-day records.
	var legacy int
	if err = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='health_samples'").Scan(&legacy); err != nil {
		return err
	}
	if legacy > 0 {
		rows, e := tx.Query("SELECT record_id,fetched_at,payload_json FROM health_samples ORDER BY fetched_at")
		if e != nil {
			return e
		}
		type oldSample struct{ id, at, body string }
		var samples []oldSample
		for rows.Next() {
			var sample oldSample
			if e = rows.Scan(&sample.id, &sample.at, &sample.body); e != nil {
				rows.Close()
				return e
			}
			samples = append(samples, sample)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		merged := map[string]oldSample{}
		for _, sample := range samples {
			var old map[string]any
			if decodeObject([]byte(sample.body), &old) != nil {
				continue
			}
			safe, _ := sanitizePayload(old)
			for day, value := range safe {
				data, e := canonicalJSON(value)
				if e != nil {
					return e
				}
				merged[sample.id+"\x00"+day] = oldSample{sample.id, sample.at, string(data)}
			}
		}
		for key, sample := range merged {
			day := strings.SplitN(key, "\x00", 2)[1]
			if _, err = tx.Exec("INSERT OR IGNORE INTO health_data(user_id,date,data,created_at,updated_at) VALUES(?,?,?,?,?)", sample.id, day, sample.body, sample.at, sample.at); err != nil {
				return err
			}
		}
	}
	for _, day := range sortedKeys(payload) {
		data, e := canonicalJSON(payload[day])
		if e != nil {
			return e
		}
		_, err = tx.Exec(`INSERT INTO health_data(user_id,date,data,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(user_id,date) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at`, userID, day, string(data), at, at)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func storeJSON(path string, envelope map[string]any) error {
	if err := outputParent(path, true); err != nil {
		return err
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	file, err := openPrivate(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = restrictPath(path, false); err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	if err != nil {
		return err
	}
	return file.Sync()
}

type sample struct {
	userID, date string
	data         map[string]any
}

func loadSamples(kind, path, start string) ([]sample, error) {
	var result []sample
	if kind == "sqlite" {
		db, err := openDatabase(path, false)
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		defer db.Close()
		rows, err := db.Query("SELECT user_id,date,data FROM health_data WHERE date>=? ORDER BY date,id", start)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var s sample
			var data string
			if err = rows.Scan(&s.userID, &s.date, &data); err != nil {
				return nil, err
			}
			if decodeObject([]byte(data), &s.data) != nil {
				continue
			}
			result = append(result, s)
		}
		return result, rows.Err()
	}
	file, err := openPrivate(path, os.O_RDONLY, 0600)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err = restrictPath(path, false); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 64<<20)
	for scanner.Scan() {
		var row map[string]any
		if decodeObject(scanner.Bytes(), &row) != nil {
			continue
		}
		payload, ok := row["payload"].(map[string]any)
		if !ok {
			continue
		}
		id := first(textValue(row, "user_id"), textValue(row, "record_id"))
		for _, day := range sortedKeys(payload) {
			if day < start {
				continue
			}
			value, ok := payload[day].(map[string]any)
			if ok {
				result = append(result, sample{id, day, value})
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading NDJSON: %w", err)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].date < result[j].date })
	return result, nil
}
