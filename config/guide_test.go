package config

import (
	"strings"
	"testing"
)

func TestGuideConfiguration(t *testing.T) {
	cfg := defaultConfig()
	if err := applyINI(&cfg, strings.NewReader("[guide]\nserver_root = /server/custom\nmode = pre-re\n")); err != nil {
		t.Fatal(err)
	}
	if cfg.GuideServerRoot != "/server/custom" || cfg.GuideMode != "pre-re" {
		t.Fatal(cfg.GuideServerRoot, cfg.GuideMode)
	}
	if err := parseCLI(&cfg, []string{"--guide-server-root", "/another", "--guide-mode", "re"}); err != nil {
		t.Fatal(err)
	}
	if cfg.GuideServerRoot != "/another" || cfg.GuideMode != "re" {
		t.Fatal("CLI precedence")
	}
}
