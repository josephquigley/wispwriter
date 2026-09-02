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
	"net/url"
	"strings"

	"github.com/writeas/web-core/activitystreams"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// imageAttachments builds a post's ActivityPub attachment list.
//
// Mastodon strips <img> out of an object's content and renders media only from
// the attachment array, so an image that appears solely inline is invisible
// there. Attaching it is what makes it display.
//
// Images are gathered from two places, deduplicated by URL and kept in order of
// appearance:
//
//   - <img> tags in the syndicated HTML. This covers uploads and any image
//     written into the body, whether by Markdown or raw HTML, and carries the
//     alt attribute along.
//   - extracted, the URLs already found in the post text by extractImages.
//     Those are bare image URLs rather than <img> tags, so HTML parsing alone
//     would miss them and they would stop being attached.
//
// altText supplies names for the extracted URLs, which have no alt attribute of
// their own. An alt attribute in the HTML wins over it.
func imageAttachments(syndicatedHTML string, extracted []string, altText map[string]string, base string) []activitystreams.Attachment {
	b, baseErr := url.Parse(base)
	resolve := func(v string) string {
		if baseErr != nil || !b.IsAbs() {
			return v
		}
		return absolutizeURL(v, b)
	}

	var attachments []activitystreams.Attachment
	seen := map[string]bool{}

	add := func(rawURL, name string) {
		resolved := resolve(strings.TrimSpace(rawURL))
		if resolved == "" || seen[resolved] {
			return
		}
		seen[resolved] = true

		a := activitystreams.NewImageAttachment(resolved)
		if name != "" {
			a.Name = name
		}
		attachments = append(attachments, a)
	}

	for _, img := range parseImages(syndicatedHTML) {
		add(img.src, img.alt)
	}
	for _, u := range extracted {
		add(u, altText[u])
	}

	return attachments
}

type parsedImage struct {
	src string
	alt string
}

// parseImages returns the src and alt of every <img> in content, in document
// order. Content that will not parse yields no images rather than an error:
// a missing attachment is better than a dropped post.
func parseImages(content string) []parsedImage {
	if content == "" {
		return nil
	}

	body := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(content), body)
	if err != nil {
		return nil
	}

	var images []parsedImage
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Img {
			var img parsedImage
			for _, attr := range n.Attr {
				switch attr.Key {
				case "src":
					img.src = attr.Val
				case "alt":
					img.alt = attr.Val
				}
			}
			if img.src != "" {
				images = append(images, img)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return images
}
