package healthsync

import (
	"os"
	"path/filepath"
	"testing"
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
