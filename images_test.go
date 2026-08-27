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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/writefreely/writefreely/config"
)

func TestUploadConfigDefaults(t *testing.T) {
	cfg := config.New()
	assert.False(t, cfg.Uploads.Enabled, "uploads are opt-in")
	assert.Equal(t, 10, cfg.Uploads.MaxSizeMB)
	assert.Equal(t, []string{"image/png", "image/jpeg", "image/gif"}, cfg.AllowedUploadTypes())
}

func TestPostImagesTableExists(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, err := app.db.Exec("SELECT id, owner_id, post_id, sha256, filename, mime, size, created FROM post_images WHERE 1 = 0")
	assert.NoError(t, err, "post_images must exist with the expected columns")
}
