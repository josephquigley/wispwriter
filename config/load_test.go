/*
 * Copyright © 2026 Joseph Quigley.
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

// TestLoadFillsDockerAssetDirs covers a configuration written for another
// container image, which leaves the asset directories empty because its
// assets sat in the working directory. Empty values here resolve against
// the state directory, where there are none, and the server exits loading
// templates.
func TestLoadFillsDockerAssetDirs(t *testing.T) {
	t.Setenv("WRITEFREELY_DOCKER", "True")
	t.Setenv("WRITEFREELY_DOCKER_PARENT_DIR", "/usr/share/writefreely")

	cfg, err := Load(writeConfig(t, "[server]\ntemplates_parent_dir =\nstatic_parent_dir =\npages_parent_dir =\n\n[app]\nhost = http://localhost:8080\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  string
	}{
		{"templates_parent_dir", cfg.Server.TemplatesParentDir},
		{"static_parent_dir", cfg.Server.StaticParentDir},
		{"pages_parent_dir", cfg.Server.PagesParentDir},
	} {
		if tc.got != "/usr/share/writefreely" {
			t.Errorf("%s = %q, want /usr/share/writefreely", tc.name, tc.got)
		}
	}
}

// TestLoadKeepsConfiguredAssetDirs makes sure the fallback above never
// overrides a directory somebody chose on purpose.
func TestLoadKeepsConfiguredAssetDirs(t *testing.T) {
	t.Setenv("WRITEFREELY_DOCKER", "True")
	t.Setenv("WRITEFREELY_DOCKER_PARENT_DIR", "/usr/share/writefreely")

	cfg, err := Load(writeConfig(t, "[server]\ntemplates_parent_dir = /go\nstatic_parent_dir = /go\npages_parent_dir = /go\n\n[app]\nhost = http://localhost:8080\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.TemplatesParentDir != "/go" {
		t.Errorf("templates_parent_dir = %q, want /go", cfg.Server.TemplatesParentDir)
	}
}

// TestLoadLeavesAssetDirsOutsideDocker keeps the fallback to containers.
// A bare metal install runs from the directory holding its assets, where
// an empty value is the correct answer.
func TestLoadLeavesAssetDirsOutsideDocker(t *testing.T) {
	t.Setenv("WRITEFREELY_DOCKER", "True")
	os.Unsetenv("WRITEFREELY_DOCKER")

	cfg, err := Load(writeConfig(t, "[app]\nhost = http://localhost:8080\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.TemplatesParentDir != "" {
		t.Errorf("templates_parent_dir = %q, want empty", cfg.Server.TemplatesParentDir)
	}
}
