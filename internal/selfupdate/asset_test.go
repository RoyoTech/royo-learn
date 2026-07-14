package selfupdate

import (
	"errors"
	"testing"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "linux", goarch: "amd64", want: "royo-learn-linux-amd64.tar.gz"},
		{goos: "linux", goarch: "arm64", want: "royo-learn-linux-arm64.tar.gz"},
		{goos: "darwin", goarch: "amd64", want: "royo-learn-darwin-amd64.tar.gz"},
		{goos: "darwin", goarch: "arm64", want: "royo-learn-darwin-arm64.tar.gz"},
		{goos: "windows", goarch: "amd64", want: "royo-learn-windows-amd64.zip"},
		{goos: "windows", goarch: "arm64", want: "royo-learn-windows-arm64.zip"},
	}

	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			got, err := AssetName(tc.goos, tc.goarch)
			if err != nil {
				t.Fatalf("AssetName(%q, %q) returned error: %v", tc.goos, tc.goarch, err)
			}
			if got != tc.want {
				t.Fatalf("AssetName(%q, %q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestAssetNameUnsupportedPlatform(t *testing.T) {
	cases := []struct {
		goos   string
		goarch string
	}{
		{goos: "plan9", goarch: "amd64"},
		{goos: "linux", goarch: "386"},
		{goos: "js", goarch: "wasm"},
	}

	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			if _, err := AssetName(tc.goos, tc.goarch); !errors.Is(err, ErrUnsupportedPlatform) {
				t.Fatalf("AssetName(%q, %q) error = %v, want ErrUnsupportedPlatform", tc.goos, tc.goarch, err)
			}
		})
	}
}

func TestBinaryName(t *testing.T) {
	if got := BinaryName("windows"); got != "royo-learn.exe" {
		t.Fatalf("BinaryName(windows) = %q, want royo-learn.exe", got)
	}
	if got := BinaryName("linux"); got != "royo-learn" {
		t.Fatalf("BinaryName(linux) = %q, want royo-learn", got)
	}
	if got := BinaryName("darwin"); got != "royo-learn" {
		t.Fatalf("BinaryName(darwin) = %q, want royo-learn", got)
	}
}
