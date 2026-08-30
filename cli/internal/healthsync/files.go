package healthsync

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func checkDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private directory must not be a link or non-directory: %s", path)
	}
	return restrictPath(path, true)
}
func secureDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return checkDir(path)
}
func readPrivate(path string, limit int64) ([]byte, error) {
	f, err := openPrivate(path, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = restrictPath(path, false); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("private file exceeds its size limit")
	}
	return data, nil
}
func writeNew(path string, data []byte) (err error) {
	f, err := openPrivate(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	if err = restrictPath(path, false); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
func atomicWrite(path string, data []byte) error {
	if err := checkDir(filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("refusing to replace a link or non-regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".healthsync-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if err = f.Close(); err != nil {
		return err
	}
	if err = restrictPath(temporary, false); err != nil {
		return err
	}
	f, err = openPrivate(temporary, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporary, path); err != nil {
		return err
	}
	// The temporary already has its final private permissions. Do not report a
	// post-commit chmod failure as though the destination had not been replaced.
	return nil
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'))
}
