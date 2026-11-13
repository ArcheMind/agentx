package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const helperFlag = "--internal-update-helper"

func replaceExecutable(stagedPath, targetPath, version string, stdout, stderr io.Writer, debug bool) (handedOff bool, err error) {
	helperDir, err := os.MkdirTemp("", "sm-update-helper-")
	if err != nil {
		return false, fmt.Errorf("create update helper directory: %w", err)
	}
	defer func() {
		if !handedOff {
			os.RemoveAll(helperDir)
		}
	}()
	executable, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("locate update helper source: %w", err)
	}
	helperPath := filepath.Join(helperDir, "sm-update-helper.exe")
	if err := copyHelper(executable, helperPath); err != nil {
		return false, err
	}
	// Inherit a handle, rather than reopening the parent by a recyclable PID.
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, true, uint32(os.Getpid()))
	if err != nil {
		return false, fmt.Errorf("open parent process for update: %w", err)
	}
	defer windows.CloseHandle(parent)
	args := []string{helperFlag, strconv.FormatUint(uint64(parent), 10), stagedPath, targetPath, version, helperDir}
	command := exec.Command(helperPath, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	command.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(parent)}}
	if debug {
		for _, value := range os.Environ() {
			if !strings.HasPrefix(strings.ToUpper(value), "SM_LOG=") {
				command.Env = append(command.Env, value)
			}
		}
		command.Env = append(command.Env, "SM_LOG=debug")
	}
	if err := command.Start(); err != nil {
		platformLog(debug, stderr, "update_helper_start", map[string]any{"executable": helperPath, "args": args, "parent_handle": parent, "parent_pid": os.Getpid()}, nil, err)
		return false, fmt.Errorf("start update helper: %w", err)
	}
	platformLog(debug, stderr, "update_helper_start", map[string]any{"executable": helperPath, "args": args, "parent_handle": parent, "parent_pid": os.Getpid()}, map[string]any{"helper_pid": command.Process.Pid}, nil)
	_ = command.Process.Release()
	return true, nil
}

func copyHelper(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open update helper source: %w", err)
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return fmt.Errorf("create update helper: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Close())
}

func RunHelper(args []string, stdout, stderr io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != helperFlag {
		return false, nil
	}
	if len(args) != 6 {
		return true, errors.New("invalid internal update helper arguments")
	}
	parentValue, err := strconv.ParseUint(args[1], 10, strconv.IntSize)
	if err != nil || parentValue == 0 {
		return true, errors.New("invalid internal update parent handle")
	}
	parent := windows.Handle(parentValue)
	defer windows.CloseHandle(parent)
	staged, target, version, helperDir := args[2], args[3], args[4], args[5]
	debug := os.Getenv("SM_LOG") == "debug"
	executable, err := os.Executable()
	if err != nil {
		return true, err
	}
	if !filepath.IsAbs(staged) || !filepath.IsAbs(target) || !filepath.IsAbs(helperDir) ||
		!strings.EqualFold(filepath.Clean(executable), filepath.Join(filepath.Clean(helperDir), "sm-update-helper.exe")) ||
		!strings.HasPrefix(filepath.Base(helperDir), "sm-update-helper-") ||
		!strings.EqualFold(filepath.Dir(staged), filepath.Dir(target)) || strings.EqualFold(staged, target) ||
		version == "" || strings.ContainsAny(version, "\r\n") {
		return true, errors.New("invalid internal update helper paths or version")
	}
	defer func() {
		if err := os.Remove(staged); err != nil && !errors.Is(err, os.ErrNotExist) {
			platformLog(debug, stderr, "update_helper_cleanup_staged", staged, nil, err)
		}
		if err := removeRunningHelper(executable); err != nil {
			platformLog(debug, stderr, "update_helper_cleanup_self", executable, nil, err)
		} else if err := os.Remove(helperDir); err != nil {
			platformLog(debug, stderr, "update_helper_cleanup_directory", helperDir, nil, err)
		}
	}()
	event, err := windows.WaitForSingleObject(parent, 30000)
	platformLog(debug, stderr, "update_helper_wait", map[string]any{"parent_handle": parent, "timeout_ms": 30000}, map[string]any{"event": event}, err)
	if err != nil {
		return true, fmt.Errorf("wait for original SessionMan process: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return true, fmt.Errorf("original SessionMan process did not exit (wait result %d)", event)
	}
	err = finishReplacement(staged, target)
	platformLog(debug, stderr, "update_helper_replace", map[string]any{"staged": staged, "target": target, "version": version}, nil, err)
	if err != nil {
		return true, err
	}
	fmt.Fprintf(stdout, "Updated SessionMan to %s.\n", version)
	return true, nil
}

func platformLog(debug bool, writer io.Writer, event string, input, output any, err error) {
	if !debug {
		return
	}
	entry := map[string]any{"event": event, "input": input, "output": output, "error": ""}
	if err != nil {
		entry["error"] = err.Error()
	}
	_ = json.NewEncoder(writer).Encode(entry)
}

func finishReplacement(staged, target string) error {
	backup, err := os.CreateTemp(filepath.Dir(target), ".sm-update-backup-*")
	if err != nil {
		return fmt.Errorf("prepare executable backup: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(backupPath)
		return err
	}
	if err := os.Rename(target, backupPath); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("back up installed executable: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		if restoreErr := os.Rename(backupPath, target); restoreErr != nil {
			return fmt.Errorf("replace executable: %w; restore failed: %v; original executable retained at %s", err, restoreErr, backupPath)
		}
		return fmt.Errorf("replace executable (original restored): %w", err)
	}
	_ = os.Remove(backupPath)
	return nil
}

func removeRunningHelper(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	return windows.SetFileInformationByHandle(handle, windows.FileDispositionInfoEx, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)))
}

func withCacheLock(path string, fn func() error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
		return err
	}
	defer windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	return fn()
}
