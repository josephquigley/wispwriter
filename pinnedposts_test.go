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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// pinnedPositions returns the collection's pinned post IDs in nav order,
// paired with their stored positions, for assertions.
func pinnedPositions(t *testing.T, app *App, collID int64) ([]string, []int64) {
	t.Helper()
	rows, err := app.db.Query("SELECT id, pinned_position FROM posts WHERE collection_id = ? AND pinned_position IS NOT NULL ORDER BY pinned_position ASC, created ASC", collID)
	assert.NoError(t, err)
	defer rows.Close()

	ids := []string{}
	positions := []int64{}
	for rows.Next() {
		var id string
		var pos int64
		assert.NoError(t, rows.Scan(&id, &pos))
		ids = append(ids, id)
		positions = append(positions, pos)
	}
	return ids, positions
}

// seedPinnedPosts creates one post per given position in the collection and
// writes that pinned_position straight to the database, so sparse and
// duplicate sequences can be constructed the way existing blogs have them.
// Each post gets a distinct, ascending creation time so the tiebreak between
// duplicate positions is deterministic. It returns the post IDs in creation
// order.
func seedPinnedPosts(t *testing.T, app *App, coll *Collection, positions []int64) []string {
	t.Helper()

	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{}
	for i, pos := range positions {
		title := fmt.Sprintf("Pinned %d", i+1)
		content := fmt.Sprintf("Body of pinned post %d.", i+1)
		p, err := app.db.CreatePost(coll.OwnerID, coll.ID, &SubmittedPost{Title: &title, Content: &content})
		if err != nil {
			t.Fatalf("create pinned post: %v", err)
		}
		created := base.Add(time.Duration(i) * time.Minute).Format("2006-01-02 15:04:05")
		if _, err := app.db.Exec("UPDATE posts SET pinned_position = ?, created = ? WHERE id = ?", pos, created, p.ID); err != nil {
			t.Fatalf("seed pinned position: %v", err)
		}
		ids = append(ids, p.ID)
	}
	return ids
}

func TestNormalizePinnedPositionsClosesGaps(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "gappy")

	ids := seedPinnedPosts(t, app, coll, []int64{1, 5, 9})

	assert.NoError(t, app.db.NormalizePinnedPositions(coll.ID))

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs, "order must be preserved")
	assert.Equal(t, []int64{1, 2, 3}, gotPos)
}

func TestNormalizePinnedPositionsResolvesDuplicates(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "dupey")

	ids := seedPinnedPosts(t, app, coll, []int64{1, 1, 2})

	assert.NoError(t, app.db.NormalizePinnedPositions(coll.ID))

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs, "duplicates break ties by creation time")
	assert.Equal(t, []int64{1, 2, 3}, gotPos)
}

func TestNormalizePinnedPositionsIsIdempotent(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "idem")

	seedPinnedPosts(t, app, coll, []int64{1, 5, 9})

	assert.NoError(t, app.db.NormalizePinnedPositions(coll.ID))
	firstIDs, firstPos := pinnedPositions(t, app, coll.ID)

	assert.NoError(t, app.db.NormalizePinnedPositions(coll.ID))
	secondIDs, secondPos := pinnedPositions(t, app, coll.ID)

	assert.Equal(t, firstIDs, secondIDs)
	assert.Equal(t, firstPos, secondPos)
}

func TestNormalizePinnedPositionsWithNoPinnedPosts(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "nopins")

	assert.NoError(t, app.db.NormalizePinnedPositions(coll.ID))
}

func TestSwapPinnedPositions(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "swapper")

	ids := seedPinnedPosts(t, app, coll, []int64{1, 2, 3})
	assert.NoError(t, app.db.SwapPinnedPositions(coll.ID, u.ID, ids[0], ids[1]))

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, []string{ids[1], ids[0], ids[2]}, gotIDs)
	assert.Equal(t, []int64{1, 2, 3}, gotPos, "positions stay dense")
}

func TestSwapPinnedPositionsRejectsForeignPost(t *testing.T) {
	app, _ := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "swapowner")
	_, otherColl, _ := createTemplateTestUser(t, app, "swapstranger")

	ids := seedPinnedPosts(t, app, coll, []int64{1, 2})
	otherIDs := seedPinnedPosts(t, app, otherColl, []int64{1})

	err := app.db.SwapPinnedPositions(coll.ID, u.ID, ids[0], otherIDs[0])
	assert.Error(t, err, "a post from another blog must be rejected")

	_, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, []int64{1, 2}, gotPos, "nothing may be written on rejection")
}
