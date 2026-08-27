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

// TestForkIdentity pins which surfaces carry the fork's edition name and
// which must keep identifying as plain WriteFreely to other machines.
func TestForkIdentity(t *testing.T) {
	assert.Equal(t, "WriteFreely (Colophon Edition)", FormatSoftwareName())
	assert.Equal(t, "WriteFreely (Colophon Edition) "+softwareVer, FormatVersion())

	// Machine-facing identity is deliberately unchanged: NodeInfo reports
	// strings.ToLower(serverSoftware), and the User-Agent and Server header
	// use serverSoftware directly. A fork advertising itself under a
	// different name would misreport to fediverse crawlers.
	assert.Equal(t, "WriteFreely", serverSoftware)
	assert.NotContains(t, serverSoftware, "(")

	// Documentation links must resolve to a release that exists upstream.
	assert.NotEmpty(t, upstreamVer)
	// The footer offers this build's source, which the AGPL requires to be
	// the modified source rather than upstream's.
	assert.NotContains(t, softwareRepoURL, "writefreely/writefreely",
		"the source link must point at this fork, not upstream")
	assert.NotEqual(t, softwareVer, upstreamVer,
		"the fork versions independently, so these should differ")
}
