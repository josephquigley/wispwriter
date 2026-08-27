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

import "testing"

func TestDockerAssetParentDir(t *testing.T) {
	t.Run("defaults to the historical location", func(t *testing.T) {
		t.Setenv("WRITEFREELY_DOCKER_PARENT_DIR", "")
		if got := dockerAssetParentDir(); got != "/usr/share/writefreely" {
			t.Errorf("got %q, want /usr/share/writefreely", got)
		}
	})

	t.Run("honors the image's declared layout", func(t *testing.T) {
		t.Setenv("WRITEFREELY_DOCKER_PARENT_DIR", "/go")
		if got := dockerAssetParentDir(); got != "/go" {
			t.Errorf("got %q, want /go", got)
		}
	})
}
