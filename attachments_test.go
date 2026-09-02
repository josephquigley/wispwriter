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
	"testing"
)

const attachBase = "https://quigs.blog/rel-img-test"

func TestImageAttachmentsFromUploadedImage(t *testing.T) {
	html := `<p><img src="https://quigs.blog/uploads/2026/09/02/pic.png" alt="a picture"></p>`

	got := imageAttachments(html, nil, nil, attachBase)

	if len(got) != 1 {
		t.Fatalf("wanted 1 attachment, got %d: %+v", len(got), got)
	}
	if got[0].URL != "https://quigs.blog/uploads/2026/09/02/pic.png" {
		t.Errorf("wrong url: %s", got[0].URL)
	}
	if got[0].Type != "Image" {
		t.Errorf("wrong type: %s", got[0].Type)
	}
	if got[0].MediaType != "image/png" {
		t.Errorf("wrong media type: %s", got[0].MediaType)
	}
	if got[0].Name != "a picture" {
		t.Errorf("alt text should become the attachment name, got %q", got[0].Name)
	}
}

// A relative src should never reach this function post-absolutization, but
// resolving defensively costs nothing and keeps the attachment valid if some
// other caller passes raw rendered HTML.
func TestImageAttachmentsResolveRelativeSrc(t *testing.T) {
	html := `<img src="/uploads/2026/09/02/pic.png">`

	got := imageAttachments(html, nil, nil, attachBase)

	if len(got) != 1 || got[0].URL != "https://quigs.blog/uploads/2026/09/02/pic.png" {
		t.Fatalf("wanted absolute upload url, got %+v", got)
	}
}

// Bare image URLs sitting in the post text are attached today via p.Images,
// even though they are not <img> tags. That behavior must survive.
func TestImageAttachmentsKeepsTextExtractedImages(t *testing.T) {
	extracted := []string{"https://example.com/photo.jpg"}
	alts := map[string]string{"https://example.com/photo.jpg": "a photo"}

	got := imageAttachments("<p>no img tags here</p>", extracted, alts, attachBase)

	if len(got) != 1 {
		t.Fatalf("wanted 1 attachment, got %d: %+v", len(got), got)
	}
	if got[0].URL != "https://example.com/photo.jpg" {
		t.Errorf("wrong url: %s", got[0].URL)
	}
	if got[0].Name != "a photo" {
		t.Errorf("wanted alt text from the markdown map, got %q", got[0].Name)
	}
}

func TestImageAttachmentsDeduplicates(t *testing.T) {
	html := `<img src="https://example.com/photo.jpg" alt="from html">`
	extracted := []string{"https://example.com/photo.jpg"}

	got := imageAttachments(html, extracted, nil, attachBase)

	if len(got) != 1 {
		t.Fatalf("wanted 1 attachment, got %d: %+v", len(got), got)
	}
	if got[0].Name != "from html" {
		t.Errorf("the html alt should win, got %q", got[0].Name)
	}
}

func TestImageAttachmentsPreservesOrder(t *testing.T) {
	html := `<img src="https://quigs.blog/a.png"><img src="https://quigs.blog/b.png">`

	got := imageAttachments(html, []string{"https://example.com/c.jpg"}, nil, attachBase)

	wantOrder := []string{
		"https://quigs.blog/a.png",
		"https://quigs.blog/b.png",
		"https://example.com/c.jpg",
	}
	if len(got) != len(wantOrder) {
		t.Fatalf("wanted %d attachments, got %d: %+v", len(wantOrder), len(got), got)
	}
	for i, want := range wantOrder {
		if got[i].URL != want {
			t.Errorf("position %d: wanted %s, got %s", i, want, got[i].URL)
		}
	}
}

func TestImageAttachmentsWithNoImages(t *testing.T) {
	if got := imageAttachments("<p>Just words.</p>", nil, nil, attachBase); len(got) != 0 {
		t.Errorf("wanted no attachments, got %+v", got)
	}
}

func TestImageAttachmentsOmitsEmptyAltText(t *testing.T) {
	got := imageAttachments(`<img src="https://quigs.blog/a.png" alt="">`, nil, nil, attachBase)

	if len(got) != 1 {
		t.Fatalf("wanted 1 attachment, got %d", len(got))
	}
	if got[0].Name != "" {
		t.Errorf("wanted empty name, got %q", got[0].Name)
	}
}

// TestActivityObjectAttachesUploadedImages is the wiring test: an uploaded
// image referenced relatively in the body must reach the ActivityPub object as
// an attachment, because Mastodon strips <img> from content and renders media
// only from the attachment array.
func TestActivityObjectAttachesUploadedImages(t *testing.T) {
	app, p := syndicatedPost("https://quigs.blog")
	p.HTMLContent = template.HTML(`<p><img src="/uploads/2026/09/02/pic.png" alt="pic"></p>`)

	o := p.ActivityObject(app)

	if len(o.Attachment) != 1 {
		t.Fatalf("wanted 1 attachment, got %d: %+v", len(o.Attachment), o.Attachment)
	}
	if o.Attachment[0].URL != "https://quigs.blog/uploads/2026/09/02/pic.png" {
		t.Errorf("wanted an absolute upload url, got %s", o.Attachment[0].URL)
	}
	if o.Attachment[0].Name != "pic" {
		t.Errorf("wanted alt text carried through, got %q", o.Attachment[0].Name)
	}
}
