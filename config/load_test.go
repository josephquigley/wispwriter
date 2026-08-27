/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.ini")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// TestLoadDefaultsTheme covers a configuration that omits the theme key.
// Load maps onto a zero-valued Config, so the key would otherwise stay
// empty, and the base template builds the stylesheet URL from it -- an
// instance would request "/css/.css" and render with no styling at all.
func TestLoadDefaultsTheme(t *testing.T) {
	cfg, err := Load(writeConfig(t, "[app]\nhost = http://localhost:8080\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.App.Theme != DefaultTheme {
		t.Errorf("theme = %q, want %q", cfg.App.Theme, DefaultTheme)
	}
}

func TestLoadKeepsConfiguredTheme(t *testing.T) {
	cfg, err := Load(writeConfig(t, "[app]\nhost = http://localhost:8080\ntheme = coffee\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.App.Theme != "coffee" {
		t.Errorf("theme = %q, want coffee", cfg.App.Theme)
	}
}
