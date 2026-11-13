package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s := New("1.2.3", false, io.Discard)
	s.cachePath = filepath.Join(t.TempDir(), "update-check.json")
	return s
}

func testRelease(s *Service, base string) Release {
	name, _ := archiveName(s.goos, s.goarch)
	return Release{Tag: "v1.3.0", Assets: []Asset{{Name: name, URL: base + "/archive"}, {Name: "checksums.txt", URL: base + "/checksums"}}}
}

func TestStableVersionComparison(t *testing.T) {
	for _, test := range []struct {
		next, current string
		want          bool
	}{
		{"v1.10.0", "1.9.9", true}, {"2.0.0", "v1.99.99", true},
		{"1.2.3", "v1.2.3", false}, {"1.2.2", "1.2.3", false},
		{"v2.0.0-rc.1", "1.2.3", false}, {"2.0.0", "dev", false},
		{"2.0.0", "1.2.3+dirty", false}, {"v01.3.0", "1.2.3", false},
		{"9999999999999999999999.0.0", "1.2.3", false},
	} {
		if got := newer(test.next, test.current); got != test.want {
			t.Errorf("newer(%q, %q) = %v", test.next, test.current, got)
		}
	}
}

func TestPlatformArchiveNames(t *testing.T) {
	for platform, expected := range map[string]string{
		"darwin/amd64": "sessionman_Darwin_x86_64.tar.gz", "darwin/arm64": "sessionman_Darwin_arm64.tar.gz",
		"linux/amd64": "sessionman_Linux_x86_64.tar.gz", "linux/arm64": "sessionman_Linux_arm64.tar.gz",
		"windows/amd64": "sessionman_Windows_x86_64.zip", "windows/arm64": "sessionman_Windows_arm64.zip",
	} {
		parts := strings.Split(platform, "/")
		got, err := archiveName(parts[0], parts[1])
		if err != nil || got != expected {
			t.Errorf("archiveName(%s) = %q, %v", platform, got, err)
		}
	}
	if _, err := archiveName("linux", "386"); err == nil {
		t.Fatal("unsupported architecture accepted")
	}
}

func TestCheckOwnEndpointRedirectAndCache(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/release", http.StatusFound)
			return
		}
		if r.UserAgent() != "SessionMan/"+s.version {
			t.Errorf("user agent = %q", r.UserAgent())
		}
		json.NewEncoder(w).Encode(testRelease(s, base))
	}))
	defer server.Close()
	base = server.URL
	s.endpoint = base + "/releases/latest"
	for i := 0; i < 2; i++ {
		release, err := s.Check(context.Background())
		if err != nil || release == nil || release.Tag != "v1.3.0" {
			t.Fatalf("check = %#v, %v", release, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("cache made extra requests: %d", calls.Load())
	}
	s.version = "1.3.0"
	if release, err := s.Check(context.Background()); err != nil || release != nil {
		t.Fatalf("installed version still offers update: %#v, %v", release, err)
	}
	s.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	if _, err := s.Check(context.Background()); err != nil || calls.Load() != 4 {
		t.Fatalf("expired cache did not refresh: calls=%d, error=%v", calls.Load(), err)
	}
}

func TestReleaseMustBePublishedForCurrentPlatform(t *testing.T) {
	s := testService(t)
	for _, kind := range []string{"draft", "prerelease", "missing archive", "missing checksum", "tag only", "older"} {
		t.Run(kind, func(t *testing.T) {
			r := testRelease(s, "https://example.test")
			switch kind {
			case "draft":
				r.Draft = true
			case "prerelease":
				r.Prerelease = true
			case "missing archive":
				r.Assets = r.Assets[1:]
			case "missing checksum":
				r.Assets = r.Assets[:1]
			case "tag only":
				r.Assets = nil
			case "older":
				r.Tag = "v1.2.2"
			}
			if s.available(&r) != nil {
				t.Fatal("unavailable release offered")
			}
		})
	}
}

func TestCheckClaimsOnceAcrossConcurrentStarts(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(testRelease(s, "https://example.test"))
	}))
	defer server.Close()
	s.endpoint = server.URL
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			s.Check(context.Background())
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent starts made %d requests", calls.Load())
	}
}

func TestFailureIsThrottledAndDoesNotClaimLatest(t *testing.T) {
	s := testService(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "rate limit", http.StatusTooManyRequests)
	}))
	defer server.Close()
	s.endpoint = server.URL
	if _, err := s.Check(context.Background()); err == nil {
		t.Fatal("HTTP failure accepted")
	}
	if release, err := s.Check(context.Background()); err != nil || release != nil {
		t.Fatalf("failed lookup cached as a release: %#v, %v", release, err)
	}
	saved, err := s.readCache()
	if err != nil || !saved.CheckedAt.IsZero() || calls.Load() != 1 {
		t.Fatalf("failure state = %#v, calls=%d, error=%v", saved, calls.Load(), err)
	}
}

func TestCheckCancellationCoversRedirect(t *testing.T) {
	s := testService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/slow", http.StatusFound)
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	s.endpoint = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := s.Check(ctx); err == nil {
		t.Fatal("cancelled redirect succeeded")
	}
}

func TestInvalidCacheAndDevelopmentVersionSkipNetwork(t *testing.T) {
	s := testService(t)
	s.endpoint = ":invalid"
	if err := os.WriteFile(s.cachePath, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Check(context.Background()); err == nil {
		t.Fatal("invalid cache accepted")
	}
	s.version = "dev"
	if release, err := s.Check(context.Background()); err != nil || release != nil {
		t.Fatalf("development build checked: %#v %v", release, err)
	}
}

func TestHTTPDebugPreservesRawResponseAndError(t *testing.T) {
	s := testService(t)
	var log bytes.Buffer
	s.debug, s.log = true, &log
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Source", "origin")
		http.Error(w, "original error", http.StatusForbidden)
	}))
	defer server.Close()
	s.endpoint = server.URL
	s.Check(context.Background())
	var entry struct {
		Body   []byte `json:"body_base64"`
		Status int    `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(log.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if string(entry.Body) != "original error\n" || entry.Status != 403 || entry.Error == "" || !strings.Contains(log.String(), server.URL) {
		t.Fatalf("raw debug response missing: %s", log.String())
	}
}

func makeArchive(t *testing.T, goos, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if goos == "windows" {
		writer := zip.NewWriter(&buffer)
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Write(content)
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		compressed := gzip.NewWriter(&buffer)
		writer := tar.NewWriter(compressed)
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		writer.Write(content)
		writer.Close()
		compressed.Close()
	}
	return buffer.Bytes()
}

func TestExtractOnlyRegularRootExecutable(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			name := "sm"
			if goos == "windows" {
				name += ".exe"
			}
			for _, path := range []string{name, "../" + name, "folder/" + name} {
				var output bytes.Buffer
				err := extractExecutable(makeArchive(t, goos, path, []byte("executable")), goos, &output)
				if path == name {
					if err != nil || output.String() != "executable" {
						t.Fatalf("valid executable = %q, %v", output.String(), err)
					}
				} else if err == nil || output.Len() != 0 {
					t.Fatalf("unsafe entry extracted: %s", path)
				}
			}
		})
	}
}

func TestInstallVerificationAndReplacement(t *testing.T) {
	for _, kind := range []string{"checksum mismatch", "checksum missing", "invalid archive", "success"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "success" && runtime.GOOS == "windows" {
				t.Skip("Windows helper replacement is covered by platform tests")
			}
			s := testService(t)
			executable := "sm"
			if s.goos == "windows" {
				executable += ".exe"
			}
			archive := makeArchive(t, s.goos, executable, []byte("new executable"))
			if kind == "invalid archive" {
				archive = []byte("not an archive")
			}
			digest := sha256.Sum256(archive)
			if kind == "checksum mismatch" {
				digest = sha256.Sum256([]byte("different"))
			}
			name, _ := archiveName(s.goos, s.goarch)
			checksumName := name
			if kind == "checksum missing" {
				checksumName = "other.tar.gz"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/checksums" {
					fmt.Fprintf(w, "%x  %s\n", digest, checksumName)
					return
				}
				w.Write(archive)
			}))
			defer server.Close()
			target := filepath.Join(t.TempDir(), executable)
			if err := os.WriteFile(target, []byte("old executable"), 0755); err != nil {
				t.Fatal(err)
			}
			_, err := s.installTo(context.Background(), testRelease(s, server.URL), target, io.Discard, io.Discard)
			if (err == nil) != (kind == "success") {
				t.Fatalf("install result: %v", err)
			}
			data, readErr := os.ReadFile(target)
			want := "old executable"
			if kind == "success" {
				want = "new executable"
			}
			if readErr != nil || string(data) != want {
				t.Fatalf("target = %q, %v", data, readErr)
			}
			leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".sm-update-*"))
			if len(leftovers) != 0 {
				t.Fatalf("staged files leaked: %v", leftovers)
			}
		})
	}
}
