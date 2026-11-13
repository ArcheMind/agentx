package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

const endpoint = "https://sessionman.dev/releases/latest"

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Service struct {
	version   string
	endpoint  string
	cachePath string
	goos      string
	goarch    string
	client    *http.Client
	now       func() time.Time
	debug     bool
	log       io.Writer
}

func New(version string, debug bool, log io.Writer) *Service {
	cachePath := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cachePath = filepath.Join(dir, "sessionman", "update-check.json")
	}
	return &Service{
		version: version, endpoint: endpoint, cachePath: cachePath,
		goos: runtime.GOOS, goarch: runtime.GOARCH,
		client: &http.Client{}, now: time.Now, debug: debug, log: log,
	}
}

type cache struct {
	LastAttempt time.Time `json:"last_attempt_at"`
	CheckedAt   time.Time `json:"checked_at"`
	Release     *Release  `json:"release,omitempty"`
}

func (s *Service) Check(ctx context.Context) (*Release, error) {
	if _, ok := versionParts(s.version); !ok {
		return nil, nil
	}
	if _, err := archiveName(s.goos, s.goarch); err != nil {
		return nil, err
	}
	if s.cachePath == "" {
		return nil, fmt.Errorf("update cache directory unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(s.cachePath), 0700); err != nil {
		return nil, err
	}
	var saved cache
	claimed := false
	err := withCacheLock(s.cachePath+".lock", func() error {
		var err error
		saved, err = s.readCache()
		if err != nil {
			return err
		}
		if !saved.LastAttempt.IsZero() && s.now().Sub(saved.LastAttempt) < 24*time.Hour {
			return nil
		}
		saved.LastAttempt = s.now()
		if err := s.writeCache(saved); err != nil {
			return err
		}
		claimed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !claimed {
		return s.available(saved.Release), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	body, err := s.get(ctx, s.endpoint, 2<<20)
	if err != nil {
		return nil, err
	}
	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("decode update release: %w", err)
	}
	if _, ok := versionParts(release.Tag); !ok || release.Draft || release.Prerelease {
		return nil, fmt.Errorf("update endpoint did not return a stable release")
	}
	// Only the claim owner writes a result; atomic replacement keeps readers independent.
	saved.Release = &release
	saved.CheckedAt = s.now()
	if err := s.writeCache(saved); err != nil {
		return nil, err
	}
	return s.available(&release), nil
}

func (s *Service) available(release *Release) *Release {
	if release == nil || release.Draft || release.Prerelease || !newer(release.Tag, s.version) {
		return nil
	}
	name, err := archiveName(s.goos, s.goarch)
	if err != nil || release.assetURL(name) == "" || release.assetURL("checksums.txt") == "" {
		return nil
	}
	return release
}

func (r Release) assetURL(name string) string {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset.URL
		}
	}
	return ""
}

func (s *Service) readCache() (cache, error) {
	var saved cache
	data, err := os.ReadFile(s.cachePath)
	if os.IsNotExist(err) {
		return saved, nil
	}
	if err != nil {
		return saved, err
	}
	err = json.Unmarshal(data, &saved)
	return saved, err
}

func (s *Service) writeCache(saved cache) error {
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.cachePath), ".update-cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.cachePath)
}

func (s *Service) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		s.logHTTP(url, nil, nil, err)
		return nil, err
	}
	req.Header.Set("User-Agent", "SessionMan/"+s.version)
	resp, err := s.client.Do(req)
	if err != nil {
		s.logHTTP(url, resp, nil, err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err == nil && int64(len(body)) > limit {
		err = fmt.Errorf("update response exceeds %d bytes", limit)
	}
	if err == nil && resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("update request returned %s", resp.Status)
	}
	s.logHTTP(url, resp, body, err)
	return body, err
}

func (s *Service) logHTTP(url string, response *http.Response, body []byte, err error) {
	if !s.debug || s.log == nil {
		return
	}
	entry := map[string]any{
		"event": "update_http", "input": map[string]any{"method": "GET", "url": url, "user_agent": "SessionMan/" + s.version},
		"body_base64": body,
	}
	if response != nil {
		entry["status"] = response.StatusCode
		entry["headers"] = response.Header
		if response.Request != nil {
			entry["response_url"] = response.Request.URL.String()
		}
	}
	if err != nil {
		entry["error"] = err.Error()
	}
	data, marshalErr := json.Marshal(entry)
	if marshalErr == nil {
		fmt.Fprintln(s.log, string(data))
	}
}

var stableVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func versionParts(version string) ([3]uint64, bool) {
	var parts [3]uint64
	match := stableVersion.FindStringSubmatch(version)
	if match == nil {
		return parts, false
	}
	for i := range parts {
		value, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return parts, false
		}
		parts[i] = value
	}
	return parts, true
}

func newer(candidate, current string) bool {
	next, nextOK := versionParts(candidate)
	previous, previousOK := versionParts(current)
	if !nextOK || !previousOK {
		return false
	}
	for i := range next {
		if next[i] != previous[i] {
			return next[i] > previous[i]
		}
	}
	return false
}

func archiveName(goos, arch string) (string, error) {
	platform := map[string]string{"darwin": "Darwin", "linux": "Linux", "windows": "Windows"}[goos]
	architecture := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[arch]
	if platform == "" || architecture == "" {
		return "", fmt.Errorf("unsupported update platform: %s/%s", goos, arch)
	}
	extension := ".tar.gz"
	if goos == "windows" {
		extension = ".zip"
	}
	return "sessionman_" + platform + "_" + architecture + extension, nil
}
