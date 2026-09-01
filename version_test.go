/*
 * Copyright © 2026 Joseph Quigley.
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
// The Makefile overrides softwareVer through -X, and when the build context
// has no usable git repository (a git worktree, for instance, whose .git is
// a file pointing outside the context) `git describe` produces nothing.
// Injecting that empty result left every version string in the application
// truncated to its prefix.
func TestSoftwareVersionIsSet(t *testing.T) {
	assert.NotEmpty(t, softwareVer, "the version must never be empty")

	v := FormatVersion()
	assert.True(t, strings.HasPrefix(v, serverSoftwareDisplay+" "), "got %q", v)
	assert.NotEqual(t, serverSoftwareDisplay+" ", v,
		"FormatVersion must carry a version, not just the product name")
	assert.False(t, strings.HasSuffix(v, " "), "got %q", v)
}

// TestForkIdentity pins which surface carries the fork's edition name and
// which must keep identifying as plain WriteFreely to other machines.
func TestForkIdentity(t *testing.T) {
	assert.Equal(t, "WriteFreely (Wisp Edition)", serverSoftwareDisplay)
	assert.Equal(t, serverSoftwareDisplay+" "+editionVersion(), FormatVersion())

	// Machine-facing identity is deliberately unchanged: nodeinfo reports
	// strings.ToLower(serverSoftware), and the User-Agent and Server header
	// use serverSoftware directly. A fork advertising itself under a
	// different name would misreport to fediverse crawlers.
	assert.Equal(t, "WriteFreely", serverSoftware)
	assert.NotContains(t, serverSoftware, "(")
}

// TestEditionVersionCarriesForkMetadata pins the version string this build
// reports to other machines. The suffix is SemVer build metadata, which is
// ignored when comparing precedence, so a peer still reads this as the
// upstream release the fork is based on.
func TestEditionVersionCarriesForkMetadata(t *testing.T) {
	assert.Equal(t, softwareVer+"+wisp", editionVersion())
	assert.Equal(t, serverSoftwareDisplay+" "+editionVersion(), FormatVersion())
	assert.Contains(t, ServerUserAgent(""), serverSoftware+"/"+editionVersion())
}
