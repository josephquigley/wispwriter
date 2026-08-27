/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSoftwareVersionIsSet guards against builds that report a bare "v".
// The Makefile overrides softwareVer through -X, and when the build
// context has no usable git repository -- a git worktree, for instance,
// whose .git is a file pointing outside the context -- `git describe`
// produces nothing. Injecting that empty result left every version string
// in the application truncated to its prefix.
func TestSoftwareVersionIsSet(t *testing.T) {
	assert.NotEmpty(t, softwareVer, "version must never be empty")
	assert.NotEqual(t, "v", "v"+softwareVer, "the rendered version must be more than a bare v")

	v := FormatVersion()
	assert.True(t, strings.HasPrefix(v, serverSoftware+" "), "got %q", v)
	assert.NotEqual(t, serverSoftware+" ", v, "FormatVersion must carry a version, not just the product name")
	assert.False(t, strings.HasSuffix(v, " "), "got %q", v)
}
