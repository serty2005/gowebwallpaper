package main

import "testing"

func TestSelectSelfUpdateAssetUsesNewerVersionFromLatestRelease(t *testing.T) {
	release := githubRelease{
		TagName: "latest",
		Assets: []githubReleaseAsset{
			{Name: "notes.txt", BrowserDownloadURL: "https://example.test/notes.txt"},
			{Name: "webwallpaper-1.2.4.exe", BrowserDownloadURL: "https://example.test/webwallpaper-1.2.4.exe"},
			{Name: "webwallpaper-1.2.3.exe", BrowserDownloadURL: "https://example.test/webwallpaper-1.2.3.exe"},
		},
	}

	asset, ok := selectSelfUpdateAsset(release, "1.2.3")

	if !ok {
		t.Fatal("expected newer update asset")
	}
	if asset.Version != "1.2.4" {
		t.Fatalf("expected version 1.2.4, got %q", asset.Version)
	}
	if asset.DownloadURL != "https://example.test/webwallpaper-1.2.4.exe" {
		t.Fatalf("unexpected download URL: %q", asset.DownloadURL)
	}
}

func TestSelectSelfUpdateAssetSkipsSameOrOlderVersions(t *testing.T) {
	release := githubRelease{
		TagName: "latest",
		Assets: []githubReleaseAsset{
			{Name: "webwallpaper-1.2.3.exe", BrowserDownloadURL: "https://example.test/webwallpaper-1.2.3.exe"},
			{Name: "webwallpaper-1.2.2.exe", BrowserDownloadURL: "https://example.test/webwallpaper-1.2.2.exe"},
		},
	}

	_, ok := selectSelfUpdateAsset(release, "1.2.3")

	if ok {
		t.Fatal("did not expect update for same or older versions")
	}
}

func TestSelectSelfUpdateAssetSkipsDevBuilds(t *testing.T) {
	release := githubRelease{
		TagName: "latest",
		Assets: []githubReleaseAsset{
			{Name: "webwallpaper-1.2.4.exe", BrowserDownloadURL: "https://example.test/webwallpaper-1.2.4.exe"},
		},
	}

	_, ok := selectSelfUpdateAsset(release, "dev")

	if ok {
		t.Fatal("did not expect update for dev build")
	}
}

func TestCompareSemanticVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "1.2.4", right: "1.2.3", want: 1},
		{left: "1.2.3", right: "1.2.3", want: 0},
		{left: "1.2.3", right: "1.3.0", want: -1},
		{left: "2.0.0", right: "10.0.0", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.left+"_vs_"+tt.right, func(t *testing.T) {
			got, err := compareSemanticVersions(tt.left, tt.right)
			if err != nil {
				t.Fatalf("compareSemanticVersions returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}
