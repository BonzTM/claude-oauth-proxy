package buildinfo

import (
	"strings"
	"testing"
)

func TestVersionAndBanner(t *testing.T) {
	if Version() == "" {
		t.Fatal("expected version to be populated")
	}
	banner := Banner("cop")
	if !strings.Contains(banner, "cop ") {
		t.Fatalf("unexpected banner: %q", banner)
	}
	if !strings.Contains(banner, commitShort) {
		t.Fatalf("expected commit in banner: %q", banner)
	}
	if !strings.Contains(Banner(""), "claude-oauth-proxy ") {
		t.Fatalf("unexpected default banner: %q", Banner(""))
	}
}
