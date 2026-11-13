//go:build !windows

package update

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func replaceExecutable(stagedPath, targetPath, version string, stdout, stderr io.Writer, debug bool) (bool, error) {
	if err := os.Rename(stagedPath, targetPath); err != nil {
		return false, fmt.Errorf("replace executable: %w", err)
	}
	return false, nil
}

func RunHelper(args []string, stdout, stderr io.Writer) (bool, error) {
	return false, nil
}

func withCacheLock(path string, fn func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return fn()
}
