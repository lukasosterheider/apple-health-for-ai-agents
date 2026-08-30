//go:build !windows

package healthsync

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openPrivate(path string, flags int, mode os.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, uint32(mode.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("sensitive path must be a regular file")
	}
	return f, nil
}
func restrictPath(path string, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing symbolic link for private state")
	}
	return os.Chmod(path, mode)
}
func lockState(path string) (*os.File, error) {
	f, err := openPrivate(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = restrictPath(path, false); err != nil {
		f.Close()
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another Health Sync command is using this state directory; retry when it finishes")
	}
	return f, nil
}
