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
	"testing"
	"time"

	"github.com/guregu/null"
	"github.com/guregu/null/zero"
	"github.com/writeas/web-core/activitystreams"
	"github.com/writefreely/writefreely/config"
)

const apVisHost = "https://quigs.blog"

// apVisPost builds the smallest PublicPost ActivityObject will serialise, at
// the given collection visibility. `body` decides the object type: content
// holding a blank line becomes an Article, a single paragraph becomes a Note,
// and both constructors set To independently, so both need covering.
func apVisPost(vis collVisibility, body string) (*App, *PublicPost) {
	cfg := config.New()
	cfg.App.Host = apVisHost

	coll := Collection{Alias: "quigs", Title: "quigs", Visibility: vis}
	coll.hostName = apVisHost

	p := &PublicPost{
		Post: &Post{
			ID:      "abc123",
			Slug:    null.NewString("visibility-test", true),
			Title:   zero.NewString("Visibility test", true),
			Content: body,
			Created: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
		},
		Collection: &CollectionObj{Collection: coll},
	}
	return &App{cfg: cfg}, p
}

const (
	apVisArticleBody = "Intro line.\n\nSecond paragraph, so this serialises as an Article.\n"
	apVisNoteBody    = "One paragraph, so this serialises as a Note."
)

func apVisContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// followersURL is the value a peer must find in to/cc to treat a post as
// followers-only. Mbin matches it against the actor's own advertised
// followers collection, so it has to be exactly FederatedAccount()+"/followers".
func apVisFollowersURL() string {
	return apVisHost + "/api/collections/quigs/followers"
}

// A public collection keeps today's addressing exactly: the magic Public
// collection in `to`, followers in `cc`. This is the regression guard — the
// visibility change must not quietly demote public blogs.
func TestActivityObjectPublicCollectionAddressesPublic(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"article", apVisArticleBody},
		{"note", apVisNoteBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, p := apVisPost(CollPublic, tc.body)

			o := p.ActivityObject(app)

			if !apVisContains(o.To, activitystreams.ToPublic) {
				t.Errorf("a public blog must address the Public collection in `to`, got to=%v", o.To)
			}
			if !apVisContains(o.CC, apVisFollowersURL()) {
				t.Errorf("a public blog must carry its followers collection in `cc`, got cc=%v", o.CC)
			}
		})
	}
}

// An unlisted collection must be followers-only on the wire.
//
// This is the assertion the whole change exists for. A peer decides
// visibility from `to` and `cc` MERGED — Mbin's containsPublicTarget() does
// exactly that — so leaving Public in `cc` would read as fully public and
// silently defeat the change. Hence: Public in neither field.
func TestActivityObjectUnlistedCollectionIsFollowersOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"article", apVisArticleBody},
		{"note", apVisNoteBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, p := apVisPost(CollUnlisted, tc.body)

			o := p.ActivityObject(app)

			merged := append(append([]string{}, o.To...), o.CC...)

			if apVisContains(merged, activitystreams.ToPublic) {
				t.Errorf("an unlisted blog must not address the Public collection in either field, got to=%v cc=%v", o.To, o.CC)
			}
			if !apVisContains(merged, apVisFollowersURL()) {
				t.Errorf("an unlisted blog must address its followers collection, got to=%v cc=%v", o.To, o.CC)
			}
			if !apVisContains(o.To, apVisFollowersURL()) {
				t.Errorf("the followers collection belongs in `to` for a followers-only post, got to=%v", o.To)
			}
		})
	}
}
