package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) Install(ctx context.Context, release Release, stdout, stderr io.Writer) (bool, error) {
	target, err := os.Executable()
	if err != nil {
		return false, err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return false, err
	}
	return s.installTo(ctx, release, target, stdout, stderr)
}

func (s *Service) installTo(ctx context.Context, release Release, target string, stdout, stderr io.Writer) (bool, error) {
	if s.available(&release) == nil {
		return false, fmt.Errorf("release is not an available update for this platform")
	}
	info, err := os.Stat(target)
	if err != nil {
		return false, err
	}
	staged, err := os.CreateTemp(filepath.Dir(target), ".sm-update-*")
	if err != nil {
		return false, fmt.Errorf("prepare update beside %s: %w", target, err)
	}
	stagedPath := staged.Name()
	owned := true
	defer func() {
		staged.Close()
		if owned {
			os.Remove(stagedPath)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	name, _ := archiveName(s.goos, s.goarch)
	checksums, err := s.get(ctx, release.assetURL("checksums.txt"), 2<<20)
	if err != nil {
		return false, fmt.Errorf("download checksums: %w", err)
	}
	expected, err := checksumFor(checksums, name)
	if err != nil {
		return false, err
	}
	archive, err := s.get(ctx, release.assetURL(name), 128<<20)
	if err != nil {
		return false, fmt.Errorf("download update: %w", err)
	}
	actual := sha256.Sum256(archive)
	if !bytes.Equal(expected, actual[:]) {
		return false, fmt.Errorf("SHA-256 verification failed for %s", name)
	}
	if err := extractExecutable(archive, s.goos, staged); err != nil {
		return false, fmt.Errorf("extract update: %w", err)
	}
	if err := staged.Chmod(info.Mode().Perm()); err != nil {
		return false, err
	}
	if err := staged.Sync(); err != nil {
		return false, err
	}
	if err := staged.Close(); err != nil {
		return false, err
	}
	handedOff, err := replaceExecutable(stagedPath, target, release.Tag, stdout, stderr, s.debug)
	if handedOff {
		owned = false
	}
	return handedOff, err
}

func checksumFor(data []byte, name string) ([]byte, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("invalid SHA-256 checksum for %s", name)
		}
		return digest, nil
	}
	return nil, fmt.Errorf("checksum for %s was not found", name)
}

func extractExecutable(data []byte, goos string, output io.Writer) error {
	if goos == "windows" {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return err
		}
		for _, file := range reader.File {
			if file.Name != "sm.exe" || !file.Mode().IsRegular() {
				continue
			}
			body, err := file.Open()
			if err != nil {
				return err
			}
			defer body.Close()
			return copyExecutable(output, body)
		}
	} else {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		defer reader.Close()
		archive := tar.NewReader(reader)
		for {
			file, err := archive.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if file.Name == "sm" && file.Typeflag == tar.TypeReg {
				return copyExecutable(output, archive)
			}
		}
	}
	return fmt.Errorf("release archive has no regular SessionMan executable")
}

func copyExecutable(output io.Writer, input io.Reader) error {
	const limit = 256 << 20
	written, err := io.Copy(output, io.LimitReader(input, limit+1))
	if err != nil {
		return err
	}
	if written == 0 || written > limit {
		return fmt.Errorf("invalid executable size: %d", written)
	}
	return nil
}
