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
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
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

func TestViewPinnedPosts(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinviewer")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2})

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	rec := assertRendersCleanly(t, router, "GET", "/me/c/"+coll.Alias+"/pinned", cookies, http.StatusOK)
	body := rec.Body.String()
	assert.Contains(t, body, "Pinned 1")
	assert.Contains(t, body, "Pinned 2")

	// The first row has no "move up" control and the last no "move down".
	base := "/me/c/" + coll.Alias + "/pinned/"
	assert.NotContains(t, body, base+ids[0]+"/up")
	assert.Contains(t, body, base+ids[0]+"/down")
	assert.Contains(t, body, base+ids[1]+"/up")
	assert.NotContains(t, body, base+ids[1]+"/down")
	assert.Contains(t, body, base+ids[0]+"/remove")
	assert.Contains(t, body, base+ids[1]+"/remove")
}

func TestViewPinnedPostsRejectsNonOwner(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "pinowner")
	other, _, _ := createTemplateTestUser(t, app, "pinintruder")

	cookies := []*http.Cookie{loginCookie(t, app, other)}
	rec, _ := renderedRequest(t, router, "GET", "/me/c/"+coll.Alias+"/pinned", cookies)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestViewPinnedPostsEmptyState(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinempty")

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	_, _ = renderedRequest(t, router, "GET", "/me/c/"+coll.Alias+"/pinned", cookies)
	rec := assertRendersCleanly(t, router, "GET", "/me/c/"+coll.Alias+"/pinned", cookies, http.StatusOK)
	assert.Contains(t, rec.Body.String(), "no pinned posts")
}

var csrfTokenRegex = regexp.MustCompile(`name="gorilla\.csrf\.Token" value="([^"]+)"`)

// pinnedPageSession loads the pinned post management page and returns the
// cookies and CSRF token needed to POST one of its forms back, exercising
// the real CSRF protection rather than disabling it.
func pinnedPageSession(t *testing.T, app *App, router *mux.Router, u *User, alias string) ([]*http.Cookie, string) {
	t.Helper()

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	rec, _ := renderedRequest(t, router, "GET", "/me/c/"+alias+"/pinned", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("load pinned page: status %d", rec.Code)
	}
	cookies = append(cookies, rec.Result().Cookies()...)

	m := csrfTokenRegex.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatal("pinned page did not render a CSRF token")
	}
	return cookies, m[1]
}

// postForm posts a form to the router with the given cookies.
func postForm(t *testing.T, router *mux.Router, path string, cookies []*http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// gorilla/csrf assumes requests are served over TLS and requires a
	// same-origin Referer, which a real browser submitting the form sends.
	req.Header.Set("Referer", "https://"+req.Host+path)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// postPinnedAction performs one of the pinned post actions with a valid CSRF
// token.
func postPinnedAction(t *testing.T, app *App, router *mux.Router, u *User, alias, postID, action string) *httptest.ResponseRecorder {
	t.Helper()

	cookies, token := pinnedPageSession(t, app, router, u, alias)
	return postForm(t, router, "/me/c/"+alias+"/pinned/"+postID+"/"+action, cookies, url.Values{"gorilla.csrf.Token": {token}})
}

func TestMovePinnedPostDown(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinmover")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2, 3})

	rec := postPinnedAction(t, app, router, u, coll.Alias, ids[0], "down")
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/me/c/"+coll.Alias+"/pinned", rec.Header().Get("Location"))

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, []string{ids[1], ids[0], ids[2]}, gotIDs)
	assert.Equal(t, []int64{1, 2, 3}, gotPos)
}

func TestMovePinnedPostUp(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinmoverup")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2, 3})

	rec := postPinnedAction(t, app, router, u, coll.Alias, ids[2], "up")
	assert.Equal(t, http.StatusFound, rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, []string{ids[0], ids[2], ids[1]}, gotIDs)
	assert.Equal(t, []int64{1, 2, 3}, gotPos)
}

func TestMovePinnedPostUpAtTopIsNoOp(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pintop")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2, 3})

	rec := postPinnedAction(t, app, router, u, coll.Alias, ids[0], "up")
	assert.Equal(t, http.StatusFound, rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs, "order is unchanged")
	assert.Equal(t, []int64{1, 2, 3}, gotPos)
}

func TestMovePinnedPostDownAtBottomIsNoOp(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinbottom")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2, 3})

	rec := postPinnedAction(t, app, router, u, coll.Alias, ids[2], "down")
	assert.Equal(t, http.StatusFound, rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs, "order is unchanged")
	assert.Equal(t, []int64{1, 2, 3}, gotPos)
}

func TestUnpinFromManagementPage(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinremover")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2, 3})

	rec := postPinnedAction(t, app, router, u, coll.Alias, ids[1], "remove")
	assert.Equal(t, http.StatusFound, rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, []string{ids[0], ids[2]}, gotIDs)
	assert.Equal(t, []int64{1, 2}, gotPos, "remaining positions stay dense")
}

func TestPinnedActionRejectsForeignPost(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinactionowner")
	_, otherColl, _ := createTemplateTestUser(t, app, "pinactionstranger")

	ids := seedPinnedPosts(t, app, coll, []int64{1, 2})
	otherIDs := seedPinnedPosts(t, app, otherColl, []int64{1})

	rec := postPinnedAction(t, app, router, u, coll.Alias, otherIDs[0], "up")
	assert.Equal(t, http.StatusNotFound, rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs)
	assert.Equal(t, []int64{1, 2}, gotPos)

	otherGotIDs, otherGotPos := pinnedPositions(t, app, otherColl.ID)
	assert.Equal(t, otherIDs, otherGotIDs)
	assert.Equal(t, []int64{1}, otherGotPos)
}

func TestPinnedActionRejectsNonOwner(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	_, coll, _ := createTemplateTestUser(t, app, "pinvictim")
	intruder, intruderColl, _ := createTemplateTestUser(t, app, "pinattacker")

	ids := seedPinnedPosts(t, app, coll, []int64{1, 2})
	seedPinnedPosts(t, app, intruderColl, []int64{1})

	cookies, token := pinnedPageSession(t, app, router, intruder, intruderColl.Alias)
	rec := postForm(t, router, "/me/c/"+coll.Alias+"/pinned/"+ids[0]+"/down", cookies, url.Values{"gorilla.csrf.Token": {token}})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs)
	assert.Equal(t, []int64{1, 2}, gotPos)
}

func TestPinnedActionRequiresCSRF(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pincsrf")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2})

	cookies, _ := pinnedPageSession(t, app, router, u, coll.Alias)
	rec := postForm(t, router, "/me/c/"+coll.Alias+"/pinned/"+ids[0]+"/down", cookies, url.Values{})
	assert.True(t, rec.Code < 200 || rec.Code >= 300, "a POST without a CSRF token must be rejected, got %d", rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs, "nothing may be reordered without a token")
	assert.Equal(t, []int64{1, 2}, gotPos)
}

func TestPinnedActionRejectsGET(t *testing.T) {
	app, router := newTemplateTestApp(t, nil)
	u, coll, _ := createTemplateTestUser(t, app, "pinget")
	ids := seedPinnedPosts(t, app, coll, []int64{1, 2})

	cookies := []*http.Cookie{loginCookie(t, app, u)}
	rec, _ := renderedRequest(t, router, "GET", "/me/c/"+coll.Alias+"/pinned/"+ids[0]+"/down", cookies)
	assert.True(t, rec.Code == http.StatusMethodNotAllowed || rec.Code == http.StatusNotFound, "a GET must never reorder, got %d", rec.Code)

	gotIDs, gotPos := pinnedPositions(t, app, coll.ID)
	assert.Equal(t, ids, gotIDs, "a GET may not reorder anything")
	assert.Equal(t, []int64{1, 2}, gotPos)
}
