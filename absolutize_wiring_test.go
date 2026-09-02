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
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/guregu/null"
	"github.com/guregu/null/zero"
	"github.com/writefreely/writefreely/config"
)

// syndicatedPost builds the smallest PublicPost that ActivityObject will
// serialise: a published collection post whose rendered HTML carries a
// root-relative image, as an uploaded image always does.
func syndicatedPost(host string) (*App, *PublicPost) {
	cfg := config.New()
	cfg.App.Host = host
	cfg.App.SingleUser = true

	coll := Collection{Alias: "quigs", Title: "quigs"}
	coll.hostName = host

	p := &PublicPost{
		Post: &Post{
			ID:          "abc123",
			Slug:        null.NewString("rel-img-test", true),
			Title:       zero.NewString("Relative image test", true),
			Content:     "Intro line.\n\n![pic](/uploads/2026/09/02/pic.png)\n",
			HTMLContent: template.HTML(`<p>Intro line.</p><p><img src="/uploads/2026/09/02/pic.png" alt="pic"></p>`),
			Created:     time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC),
		},
		Collection: &CollectionObj{Collection: coll},
	}
	return &App{cfg: cfg}, p
}

// TestActivityObjectAbsolutizesContent guards the federation half of the fix.
// A remote server resolves whatever HTML we hand it against its own address,
// so a relative image src in Note.content points every follower's instance at
// a file it does not have.
func TestActivityObjectAbsolutizesContent(t *testing.T) {
	app, p := syndicatedPost("https://quigs.blog")

	o := p.ActivityObject(app)

	want := `src="https://quigs.blog/uploads/2026/09/02/pic.png"`
	if !strings.Contains(o.Content, want) {
		t.Errorf("Note.content should carry an absolute image URL.\nwanted to find: %s\ngot: %s", want, o.Content)
	}
	if strings.Contains(o.Content, `src="/uploads`) {
		t.Errorf("Note.content still carries a relative image URL: %s", o.Content)
	}
}

// TestSyndicatedContentResolvesAgainstPermalink pins the base URL choice.
// Resolving against the site host alone would be right for root-relative URLs
// and wrong for every relative one, so the post's own permalink is the base.
func TestSyndicatedContentResolvesAgainstPermalink(t *testing.T) {
	_, p := syndicatedPost("https://quigs.blog")
	p.HTMLContent = template.HTML(`<a href="sibling">x</a>`)

	got := syndicatedContent(p, "https://quigs.blog/rel-img-test")

	want := `<a href="https://quigs.blog/sibling">x</a>`
	if got != want {
		t.Errorf("wanted %s, got %s", want, got)
	}
}

// TestRelativeURLsSurviveRenderingThenAbsolutize checks the whole chain a post
// body travels: markdown rendering, bluemonday sanitization, then
// absolutization. Sanitization runs first and could plausibly drop a relative
// URL before syndication ever sees it, which would make absolutizing moot.
func TestRelativeURLsSurviveRenderingThenAbsolutize(t *testing.T) {
	cfg := config.New()
	cfg.App.Host = "https://quigs.blog"

	rendered := applyMarkdown([]byte("![pic](pic.png)\n\n[sib](sibling)\n"), "https://quigs.blog/", cfg)
	got := absolutizeHTML(rendered, "https://quigs.blog/rel-img-test")

	for _, want := range []string{
		`src="https://quigs.blog/pic.png"`,
		`href="https://quigs.blog/sibling"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wanted to find %s\nrendered: %s\nabsolutized: %s", want, rendered, got)
		}
	}
}
