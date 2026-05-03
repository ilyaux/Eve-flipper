package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const githubLatestReleaseURL = "https://api.github.com/repos/ilyaux/Eve-flipper/releases/latest"

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type githubLatestReleaseResponse struct {
	TagName     string               `json:"tag_name"`
	HTMLURL     string               `json:"html_url"`
	PublishedAt string               `json:"published_at"`
	Assets      []githubReleaseAsset `json:"assets"`
}

type updateResolved struct {
	CurrentVersion      string
	LatestVersion       string
	HasUpdate           bool
	AutoUpdateSupported bool
	ReleaseURL          string
	PublishedAt         string
	Platform            string
	Asset               *githubReleaseAsset
	ChecksumAsset       *githubReleaseAsset
}

func normalizeAppFlavor(flavor string) string {
	flavor = strings.ToLower(strings.TrimSpace(flavor))
	if flavor == "desktop" {
		return "desktop"
	}
	return "web"
}

type updateCheckResponse struct {
	CurrentVersion      string `json:"current_version"`
	LatestVersion       string `json:"latest_version,omitempty"`
	HasUpdate           bool   `json:"has_update"`
	DismissedForSession bool   `json:"dismissed_for_session"`
	AutoUpdateSupported bool   `json:"auto_update_supported"`
	ReleaseURL          string `json:"release_url,omitempty"`
	PublishedAt         string `json:"published_at,omitempty"`
	Platform            string `json:"platform"`
	AssetName           string `json:"asset_name,omitempty"`
	ChecksumAssetName   string `json:"checksum_asset_name,omitempty"`
	CheckError          string `json:"check_error,omitempty"`
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	resolved, err := s.resolveUpdate(r.Context())
	resp := updateCheckResponse{
		CurrentVersion:      resolved.CurrentVersion,
		LatestVersion:       resolved.LatestVersion,
		HasUpdate:           resolved.HasUpdate,
		AutoUpdateSupported: resolved.AutoUpdateSupported,
		ReleaseURL:          resolved.ReleaseURL,
		PublishedAt:         resolved.PublishedAt,
		Platform:            resolved.Platform,
	}
	if resolved.HasUpdate && resolved.LatestVersion != "" && s.isUpdateDismissedForSession(userID, resolved.LatestVersion) {
		resp.DismissedForSession = true
	}
	if resolved.Asset != nil {
		resp.AssetName = resolved.Asset.Name
	}
	if resolved.ChecksumAsset != nil {
		resp.ChecksumAssetName = resolved.ChecksumAsset.Name
	}
	if err != nil {
		// Fail soft: UI can keep working if GitHub is unreachable.
		resp.CheckError = err.Error()
	}
	writeJSON(w, resp)
}

type updateSkipRequest struct {
	Version string `json:"version"`
}

func (s *Server) handleUpdateSkipForSession(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	var req updateSkipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	version := normalizeVersion(req.Version)
	if version == "" {
		writeError(w, http.StatusBadRequest, "version is required")
		return
	}
	s.setUpdateDismissedForSession(userID, version)
	writeJSON(w, map[string]any{
		"ok":      true,
		"version": version,
	})
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)
	resolved, err := s.resolveUpdate(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check latest release: "+err.Error())
		return
	}
	if !resolved.HasUpdate {
		writeJSONStatus(w, http.StatusConflict, map[string]any{
			"ok":      false,
			"message": "already up to date",
		})
		return
	}
	if resolved.Asset == nil || strings.TrimSpace(resolved.Asset.BrowserDownloadURL) == "" {
		writeError(w, http.StatusBadRequest, "auto-update is not available for this platform")
		return
	}
	if resolved.ChecksumAsset == nil || strings.TrimSpace(resolved.ChecksumAsset.BrowserDownloadURL) == "" {
		writeError(w, http.StatusBadRequest, "auto-update requires a SHA256 checksum asset for this platform")
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to locate executable: "+err.Error())
		return
	}
	exePath, _ = filepath.Abs(exePath)

	tmpExt := filepath.Ext(exePath)
	if tmpExt == "" && runtime.GOOS == "windows" {
		tmpExt = ".exe"
	}
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("eve-flipper-update-%d%s", time.Now().UnixNano(), tmpExt))

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if err := downloadFile(ctx, resolved.Asset.BrowserDownloadURL, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		writeError(w, http.StatusBadGateway, "failed to download update: "+err.Error())
		return
	}
	if err := verifyDownloadedFileChecksum(ctx, tmpPath, resolved.Asset.Name, resolved.ChecksumAsset.BrowserDownloadURL); err != nil {
		_ = os.Remove(tmpPath)
		writeError(w, http.StatusBadGateway, "failed to verify update checksum: "+err.Error())
		return
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpPath, 0o755)
	}

	scriptPath, err := writeUpdaterScript(runtime.GOOS, tmpPath, exePath)
	if err != nil {
		_ = os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, "failed to prepare updater: "+err.Error())
		return
	}
	if err := startUpdaterScript(runtime.GOOS, scriptPath); err != nil {
		_ = os.Remove(tmpPath)
		_ = os.Remove(scriptPath)
		writeError(w, http.StatusInternalServerError, "failed to launch updater: "+err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"ok":           true,
		"from_version": resolved.CurrentVersion,
		"to_version":   resolved.LatestVersion,
		"asset_name":   resolved.Asset.Name,
		"message":      "update downloaded, restarting",
	})
	s.clearUpdateDismissedForSession(userID)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	go func() {
		time.Sleep(1200 * time.Millisecond)
		os.Exit(0)
	}()
}

func normalizeUpdateSkipUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "default"
	}
	return userID
}

func (s *Server) isUpdateDismissedForSession(userID, latestVersion string) bool {
	latestVersion = normalizeVersion(latestVersion)
	if latestVersion == "" {
		return false
	}
	key := normalizeUpdateSkipUserID(userID)
	s.updateSkipMu.RLock()
	defer s.updateSkipMu.RUnlock()
	skipped := normalizeVersion(s.updateSkipByUser[key])
	return skipped != "" && skipped == latestVersion
}

func (s *Server) setUpdateDismissedForSession(userID, version string) {
	version = normalizeVersion(version)
	if version == "" {
		return
	}
	key := normalizeUpdateSkipUserID(userID)
	s.updateSkipMu.Lock()
	s.updateSkipByUser[key] = version
	s.updateSkipMu.Unlock()
}

func (s *Server) clearUpdateDismissedForSession(userID string) {
	key := normalizeUpdateSkipUserID(userID)
	s.updateSkipMu.Lock()
	delete(s.updateSkipByUser, key)
	s.updateSkipMu.Unlock()
}

func (s *Server) resolveUpdate(ctx context.Context) (updateResolved, error) {
	current := strings.TrimSpace(s.appVersion)
	if current == "" {
		current = "dev"
	}
	resp := updateResolved{
		CurrentVersion: current,
		Platform:       runtime.GOOS + "/" + runtime.GOARCH,
	}

	currentNorm := normalizeVersion(current)
	if currentNorm == "" || currentNorm == "dev" {
		return resp, nil
	}

	rel, err := s.fetchLatestRelease(ctx)
	if err != nil {
		return resp, err
	}
	latestNorm := normalizeVersion(rel.TagName)
	resp.LatestVersion = latestNorm
	resp.ReleaseURL = strings.TrimSpace(rel.HTMLURL)
	resp.PublishedAt = strings.TrimSpace(rel.PublishedAt)

	if latestNorm == "" {
		return resp, fmt.Errorf("latest release tag is empty")
	}

	resp.HasUpdate = isVersionNewer(latestNorm, currentNorm)
	resp.Asset = selectReleaseAsset(rel.Assets, runtime.GOOS, runtime.GOARCH, s.appFlavor)
	if resp.Asset != nil {
		resp.ChecksumAsset = selectChecksumAsset(rel.Assets, resp.Asset.Name)
	}
	resp.AutoUpdateSupported = resp.HasUpdate && resp.Asset != nil && resp.ChecksumAsset != nil
	return resp, nil
}

func (s *Server) fetchLatestRelease(ctx context.Context) (*githubLatestReleaseResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "eve-flipper-updater")

	client := s.updateHTTP
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("github api %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var out githubLatestReleaseResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func expectedReleaseAssetName(goos, goarch, flavor string) string {
	name := "eve-flipper"
	if normalizeAppFlavor(flavor) == "desktop" {
		name += "-desktop"
	} else {
		name += "-web"
	}
	name += fmt.Sprintf("-%s-%s", goos, goarch)
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func isAssetNameForFlavor(name, flavor string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	switch normalizeAppFlavor(flavor) {
	case "desktop":
		return strings.HasPrefix(name, "eve-flipper-desktop-")
	default:
		return strings.HasPrefix(name, "eve-flipper-web-")
	}
}

func selectReleaseAsset(assets []githubReleaseAsset, goos, goarch, flavor string) *githubReleaseAsset {
	if len(assets) == 0 {
		return nil
	}
	expected := strings.ToLower(expectedReleaseAssetName(goos, goarch, flavor))
	for i := range assets {
		name := strings.ToLower(strings.TrimSpace(assets[i].Name))
		if name == expected && isAssetNameForFlavor(name, flavor) {
			return &assets[i]
		}
	}
	pattern := strings.ToLower(fmt.Sprintf("%s-%s", goos, goarch))
	for i := range assets {
		name := strings.ToLower(strings.TrimSpace(assets[i].Name))
		if isAssetNameForFlavor(name, flavor) && strings.Contains(name, pattern) {
			return &assets[i]
		}
	}
	return nil
}

func selectChecksumAsset(assets []githubReleaseAsset, assetName string) *githubReleaseAsset {
	assetName = strings.TrimSpace(assetName)
	if assetName == "" {
		return nil
	}
	exactCandidates := map[string]bool{
		strings.ToLower(assetName + ".sha256"):     true,
		strings.ToLower(assetName + ".sha256.txt"): true,
	}
	for i := range assets {
		name := strings.ToLower(strings.TrimSpace(assets[i].Name))
		if exactCandidates[name] {
			return &assets[i]
		}
	}
	manifestCandidates := map[string]bool{
		"sha256sums":     true,
		"sha256sums.txt": true,
		"checksums.txt":  true,
	}
	for i := range assets {
		name := strings.ToLower(strings.TrimSpace(assets[i].Name))
		if manifestCandidates[name] {
			return &assets[i]
		}
	}
	return nil
}

func downloadFile(ctx context.Context, srcURL, dstPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "eve-flipper-updater")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("download http %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, res.Body); err != nil {
		return err
	}
	return nil
}

func downloadText(ctx context.Context, srcURL string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "eve-flipper-updater")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return "", fmt.Errorf("download http %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("checksum response exceeds %d bytes", maxBytes)
	}
	return string(body), nil
}

func expectedSHA256FromText(text, assetName string) (string, error) {
	assetName = strings.TrimSpace(assetName)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 1 && len(fields[0]) == sha256.Size*2 {
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return strings.ToLower(fields[0]), nil
			}
		}
		if len(fields) >= 2 && strings.TrimLeft(fields[1], "*") == assetName {
			if len(fields[0]) == sha256.Size*2 {
				if _, err := hex.DecodeString(fields[0]); err == nil {
					return strings.ToLower(fields[0]), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no SHA256 entry for %s", assetName)
}

func verifyDownloadedFileChecksum(ctx context.Context, filePath, assetName, checksumURL string) error {
	text, err := downloadText(ctx, checksumURL, 64*1024)
	if err != nil {
		return err
	}
	expected, err := expectedSHA256FromText(text, assetName)
	if err != nil {
		return err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("sha256 mismatch for %s", assetName)
	}
	return nil
}

func writeUpdaterScript(goos, srcPath, dstPath string) (string, error) {
	switch goos {
	case "windows":
		path := filepath.Join(os.TempDir(), fmt.Sprintf("eve-flipper-updater-%d.cmd", time.Now().UnixNano()))
		script := fmt.Sprintf(
			"@echo off\r\nsetlocal\r\nset \"SRC=%s\"\r\nset \"DST=%s\"\r\nfor /L %%%%i in (1,1,180) do (\r\n  move /Y \"%%SRC%%\" \"%%DST%%\" >nul 2>nul\r\n  if not exist \"%%SRC%%\" goto launch\r\n  timeout /t 1 /nobreak >nul\r\n)\r\nexit /b 1\r\n:launch\r\nstart \"\" \"%%DST%%\"\r\ndel \"%%~f0\" >nul 2>nul\r\nexit /b 0\r\n",
			escapeCmdValue(srcPath),
			escapeCmdValue(dstPath),
		)
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			return "", err
		}
		return path, nil
	default:
		path := filepath.Join(os.TempDir(), fmt.Sprintf("eve-flipper-updater-%d.sh", time.Now().UnixNano()))
		script := fmt.Sprintf(
			"#!/bin/sh\nSRC='%s'\nDST='%s'\ni=0\nwhile [ \"$i\" -lt 180 ]; do\n  if mv -f \"$SRC\" \"$DST\" 2>/dev/null; then\n    chmod +x \"$DST\" 2>/dev/null\n    nohup \"$DST\" >/dev/null 2>&1 &\n    rm -f -- \"$0\" >/dev/null 2>&1\n    exit 0\n  fi\n  i=$((i+1))\n  sleep 1\ndone\nexit 1\n",
			escapeSingleQuotes(srcPath),
			escapeSingleQuotes(dstPath),
		)
		if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
			return "", err
		}
		return path, nil
	}
}

func startUpdaterScript(goos, scriptPath string) error {
	var cmd *exec.Cmd
	if goos == "windows" {
		// Run the updater script in a child cmd process directly.
		// Using `start` here can spawn an extra console window and leave
		// a noisy "batch file cannot be found" shell behind on some hosts.
		cmd = exec.Command("cmd", "/C", scriptPath)
		hideConsoleWindow(cmd)
	} else {
		cmd = exec.Command("/bin/sh", scriptPath)
	}
	return cmd.Start()
}

func escapeCmdValue(v string) string {
	// %VAR% expansion in cmd can break absolute paths.
	return strings.ReplaceAll(v, "%", "%%")
}

func escapeSingleQuotes(v string) string {
	return strings.ReplaceAll(v, "'", "'\"'\"'")
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
	if v == "" {
		return ""
	}
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

func isVersionNewer(latest, current string) bool {
	return compareVersions(latest, current) > 0
}

type parsedVersion struct {
	core []int
	pre  []preIdentifier
}

type preIdentifier struct {
	raw      string
	isNum    bool
	numValue int
}

var gitDescribeAheadVersionPattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)*)-([0-9]+)-g[0-9a-fA-F]+(?:-dirty)?$`)

func parseGitDescribeAheadVersion(v string) (core string, commits int, ok bool) {
	v = normalizeVersion(v)
	if v == "" {
		return "", 0, false
	}
	m := gitDescribeAheadVersionPattern.FindStringSubmatch(v)
	if len(m) != 3 {
		return "", 0, false
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, false
	}
	return m[1], n, true
}

func parseVersion(v string) parsedVersion {
	v = normalizeVersion(v)
	if v == "" {
		return parsedVersion{}
	}

	var corePart, prePart string
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		corePart = v[:idx]
		prePart = v[idx+1:]
	} else {
		corePart = v
	}

	out := parsedVersion{}
	for _, part := range strings.Split(corePart, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			out.core = append(out.core, 0)
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			out.core = append(out.core, 0)
			continue
		}
		out.core = append(out.core, n)
	}

	if prePart != "" {
		for _, token := range strings.Split(prePart, ".") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if n, err := strconv.Atoi(token); err == nil {
				out.pre = append(out.pre, preIdentifier{raw: token, isNum: true, numValue: n})
			} else {
				out.pre = append(out.pre, preIdentifier{raw: strings.ToLower(token)})
			}
		}
	}

	return out
}

func compareVersions(a, b string) int {
	// git describe dev builds like 1.5.4-2-g<hash> mean "2 commits after tag 1.5.4".
	// They should be treated as newer than the plain 1.5.4 release.
	na := normalizeVersion(a)
	nb := normalizeVersion(b)
	aCore, aCommits, aAhead := parseGitDescribeAheadVersion(na)
	bCore, bCommits, bAhead := parseGitDescribeAheadVersion(nb)
	if aAhead && bAhead && aCore == bCore {
		if aCommits > bCommits {
			return 1
		}
		if aCommits < bCommits {
			return -1
		}
		return 0
	}
	if aAhead && nb == aCore {
		return 1
	}
	if bAhead && na == bCore {
		return -1
	}

	pa := parseVersion(a)
	pb := parseVersion(b)

	maxCore := len(pa.core)
	if len(pb.core) > maxCore {
		maxCore = len(pb.core)
	}
	for i := 0; i < maxCore; i++ {
		av, bv := 0, 0
		if i < len(pa.core) {
			av = pa.core[i]
		}
		if i < len(pb.core) {
			bv = pb.core[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}

	aHasPre := len(pa.pre) > 0
	bHasPre := len(pb.pre) > 0
	if !aHasPre && !bHasPre {
		return 0
	}
	if !aHasPre && bHasPre {
		return 1
	}
	if aHasPre && !bHasPre {
		return -1
	}

	maxPre := len(pa.pre)
	if len(pb.pre) > maxPre {
		maxPre = len(pb.pre)
	}
	for i := 0; i < maxPre; i++ {
		if i >= len(pa.pre) {
			return -1
		}
		if i >= len(pb.pre) {
			return 1
		}
		x := pa.pre[i]
		y := pb.pre[i]
		if x.isNum && y.isNum {
			if x.numValue > y.numValue {
				return 1
			}
			if x.numValue < y.numValue {
				return -1
			}
			continue
		}
		if x.isNum && !y.isNum {
			return -1
		}
		if !x.isNum && y.isNum {
			return 1
		}
		if x.raw > y.raw {
			return 1
		}
		if x.raw < y.raw {
			return -1
		}
	}
	return 0
}
