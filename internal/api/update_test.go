package api

import (
	"testing"

	"eve-flipper/internal/config"
	"eve-flipper/internal/esi"
)

func TestIsVersionNewer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "1.5.5", current: "1.5.4", want: true},
		{latest: "v1.5.5", current: "1.5.4", want: true},
		{latest: "1.5.4", current: "1.5.4", want: false},
		{latest: "1.5.4", current: "1.5.4-rc1", want: true},
		{latest: "1.5.4-rc2", current: "1.5.4-rc1", want: true},
		{latest: "1.5.4-rc1", current: "1.5.4", want: false},
		{latest: "1.5.4", current: "1.5.4-2-g48d7110-dirty", want: false},
		{latest: "1.5.4-3-gabc1234", current: "1.5.4-2-gabc1234", want: true},
		{latest: "1.5.4-2-gabc1234", current: "1.5.4-3-gabc1234-dirty", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.latest+"__"+tc.current, func(t *testing.T) {
			t.Parallel()
			got := isVersionNewer(tc.latest, tc.current)
			if got != tc.want {
				t.Fatalf("isVersionNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
			}
		})
	}
}

func TestSelectReleaseAsset(t *testing.T) {
	t.Parallel()

	assets := []githubReleaseAsset{
		{Name: "eve-flipper-web-linux-amd64"},
		{Name: "eve-flipper-web-linux-arm64"},
		{Name: "eve-flipper-web-windows-amd64.exe"},
		{Name: "eve-flipper-desktop-windows-amd64.exe"},
	}

	got := selectReleaseAsset(assets, "windows", "amd64", "web")
	if got == nil || got.Name != "eve-flipper-web-windows-amd64.exe" {
		t.Fatalf("windows asset mismatch: %#v", got)
	}

	got = selectReleaseAsset(assets, "linux", "arm64", "web")
	if got == nil || got.Name != "eve-flipper-web-linux-arm64" {
		t.Fatalf("linux arm64 asset mismatch: %#v", got)
	}

	got = selectReleaseAsset(assets, "darwin", "amd64", "web")
	if got != nil {
		t.Fatalf("expected nil for darwin/amd64, got %#v", got)
	}
}

func TestSelectReleaseAssetDesktopFlavor(t *testing.T) {
	t.Parallel()

	assets := []githubReleaseAsset{
		{Name: "eve-flipper-desktop-linux-amd64"},
		{Name: "eve-flipper-desktop-linux-arm64"},
		{Name: "eve-flipper-desktop-darwin-amd64"},
		{Name: "eve-flipper-desktop-darwin-arm64"},
		{Name: "eve-flipper-web-windows-amd64.exe"},
		{Name: "eve-flipper-desktop-windows-amd64.exe"},
	}

	got := selectReleaseAsset(assets, "windows", "amd64", "desktop")
	if got == nil || got.Name != "eve-flipper-desktop-windows-amd64.exe" {
		t.Fatalf("desktop windows asset mismatch: %#v", got)
	}

	got = selectReleaseAsset(assets[:1], "windows", "amd64", "desktop")
	if got != nil {
		t.Fatalf("expected nil when desktop asset missing, got %#v", got)
	}

	got = selectReleaseAsset(assets, "linux", "amd64", "desktop")
	if got == nil || got.Name != "eve-flipper-desktop-linux-amd64" {
		t.Fatalf("desktop linux amd64 asset mismatch: %#v", got)
	}

	got = selectReleaseAsset(assets, "linux", "arm64", "desktop")
	if got == nil || got.Name != "eve-flipper-desktop-linux-arm64" {
		t.Fatalf("desktop linux arm64 asset mismatch: %#v", got)
	}

	got = selectReleaseAsset(assets, "darwin", "amd64", "desktop")
	if got == nil || got.Name != "eve-flipper-desktop-darwin-amd64" {
		t.Fatalf("desktop darwin amd64 asset mismatch: %#v", got)
	}

	got = selectReleaseAsset(assets, "darwin", "arm64", "desktop")
	if got == nil || got.Name != "eve-flipper-desktop-darwin-arm64" {
		t.Fatalf("desktop darwin arm64 asset mismatch: %#v", got)
	}
}

func TestSelectChecksumAsset(t *testing.T) {
	t.Parallel()

	assets := []githubReleaseAsset{
		{Name: "eve-flipper-web-windows-amd64.exe"},
		{Name: "eve-flipper-web-windows-amd64.exe.sha256"},
		{Name: "checksums.txt"},
	}

	got := selectChecksumAsset(assets, "eve-flipper-web-windows-amd64.exe")
	if got == nil || got.Name != "eve-flipper-web-windows-amd64.exe.sha256" {
		t.Fatalf("checksum asset mismatch: %#v", got)
	}

	got = selectChecksumAsset(assets[:1], "eve-flipper-web-windows-amd64.exe")
	if got != nil {
		t.Fatalf("expected nil when checksum asset is missing, got %#v", got)
	}

	manifestOnly := []githubReleaseAsset{
		{Name: "eve-flipper-web-windows-amd64.exe"},
		{Name: "SHA256SUMS.txt"},
	}
	got = selectChecksumAsset(manifestOnly, "eve-flipper-web-windows-amd64.exe")
	if got == nil || got.Name != "SHA256SUMS.txt" {
		t.Fatalf("manifest checksum asset mismatch: %#v", got)
	}
}

func TestExpectedSHA256FromText(t *testing.T) {
	t.Parallel()

	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	got, err := expectedSHA256FromText(hash+"  eve-flipper-web-linux-amd64\n", "eve-flipper-web-linux-amd64")
	if err != nil {
		t.Fatalf("expectedSHA256FromText: %v", err)
	}
	if got != hash {
		t.Fatalf("hash = %q, want %q", got, hash)
	}

	got, err = expectedSHA256FromText(hash+"\n", "anything")
	if err != nil {
		t.Fatalf("expected single-hash checksum to parse: %v", err)
	}
	if got != hash {
		t.Fatalf("single hash = %q, want %q", got, hash)
	}
}

func TestUpdateDismissedForSessionMemory(t *testing.T) {
	t.Parallel()

	s := NewServer(config.Default(), &esi.Client{}, nil, nil, nil)
	userID := "user-1"
	latest := "1.5.4"

	if s.isUpdateDismissedForSession(userID, latest) {
		t.Fatalf("expected false before skip")
	}
	s.setUpdateDismissedForSession(userID, latest)
	if !s.isUpdateDismissedForSession(userID, latest) {
		t.Fatalf("expected true after skip")
	}
	s.clearUpdateDismissedForSession(userID)
	if s.isUpdateDismissedForSession(userID, latest) {
		t.Fatalf("expected false after clear")
	}
}
