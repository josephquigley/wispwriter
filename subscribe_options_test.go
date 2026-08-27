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
)

func TestSubscribeTogglesDefaultOn(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "subdefaults")

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.True(t, loaded.ShowSubscribeIndex, "absent attribute means on")
	assert.True(t, loaded.ShowSubscribePosts, "absent attribute means on")
}

func TestSubscribeTogglesPersist(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "subpersist")

	off := false
	on := true
	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:            uint64(u.ID),
		Alias:              &coll.Alias,
		ShowSubscribeIndex: &off,
		ShowSubscribePosts: &on,
	}, coll.Alias)
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.False(t, loaded.ShowSubscribeIndex)
	assert.True(t, loaded.ShowSubscribePosts)
}

func TestOmittedSubscribeFieldsLeaveValuesAlone(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "subomit")

	off := false
	err := app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID:            uint64(u.ID),
		Alias:              &coll.Alias,
		ShowSubscribeIndex: &off,
	}, coll.Alias)
	assert.NoError(t, err)

	// A later update that mentions neither field must not reset them.
	title := "Renamed"
	err = app.db.UpdateCollection(app, &SubmittedCollection{
		OwnerID: uint64(u.ID),
		Alias:   &coll.Alias,
		Title:   &title,
	}, coll.Alias)
	assert.NoError(t, err)

	loaded, err := app.db.GetCollection(coll.Alias)
	assert.NoError(t, err)
	assert.False(t, loaded.ShowSubscribeIndex, "an unrelated update must not flip the toggle")
	assert.True(t, loaded.ShowSubscribePosts)
}
