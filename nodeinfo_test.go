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
	"github.com/writefreely/writefreely/config"
)

// TestNodeInfoIdentifiesTheForkWithoutRenamingTheSoftware covers what a peer
// sees when it asks who we are.
//
// software.name stays "writefreely" because this is WriteFreely, patched.
// Fediverse crawlers record that name and some branch on it, so a novel name
// would make this instance unrecognised software rather than honest software.
// The version suffix and the repository link carry the fork identity instead.
func TestNodeInfoIdentifiesTheForkWithoutRenamingTheSoftware(t *testing.T) {
	cfg := config.New()
	cfg.App.SiteName = "Test Instance"
	// Multi-user, so nodeInfoConfig has no reason to reach for the database.
	cfg.App.SingleUser = false

	ni := nodeInfoConfig(nil, cfg)

	assert.Equal(t, "writefreely", ni.Software.Name)
	assert.Equal(t, editionVersion(), ni.Software.Version)
	assert.True(t, strings.HasSuffix(ni.Software.Version, "+wisp"),
		"got %q", ni.Software.Version)
	assert.Equal(t, "https://github.com/josephquigley/writefreely-wisp",
		ni.Metadata.Software.GitHub)
}
