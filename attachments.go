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

// absoluteImageURLs returns every image the post references, as absolute URLs.
//
// og:image, twitter:image and the image sitemap all need absolute URLs: a
// preview scraper or a search crawler fetches the page out of context and will
// not resolve a relative one. p.Images alone is not enough, because
// extractImages finds URLs with extract.ExtractUrls, which only recognises
// those carrying a domain — an uploaded image, referenced as /uploads/...,
// is invisible to it.
//
// Markdown image targets are gathered here instead, filtered to image-looking
// paths, resolved against base, and unioned with the already-extracted URLs.
// Order of appearance is preserved and duplicates dropped.
//
// A relative URL is dropped when base is unusable rather than emitted raw:
// consumers fall back to the blog avatar when there is no image, which is
// better than an invalid one.
func absoluteImageURLs(content string, extracted []string, base string) []string {
	b, err := url.Parse(base)
	baseUsable := err == nil && b.IsAbs()

	var out []string
	seen := map[string]bool{}

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		ref, err := url.Parse(raw)
		if err != nil {
			return
		}
		if !ref.IsAbs() {
			if !baseUsable {
				return
			}
			ref = b.ResolveReference(ref)
		}
		resolved := ref.String()
		if seen[resolved] {
			return
		}
		seen[resolved] = true
		out = append(out, resolved)
	}

	for _, m := range imageMarkdownRegex.FindAllStringSubmatch(content, -1) {
		target := m[2]
		u, err := url.Parse(strings.TrimSpace(target))
		if err != nil || !imageURLRegex.MatchString(u.Path) {
			continue
		}
		add(target)
	}
	for _, u := range extracted {
		add(u)
	}

	return out
}

// AbsoluteImages returns the post's images as absolute URLs, resolved against
// base — the collection's canonical URL at every call site.
func (p *PublicPost) AbsoluteImages(base string) []string {
	return absoluteImageURLs(p.Content, p.Images, base)
}

// AbsoluteImages returns the post's images as absolute URLs, resolved against
// base — the site host for a standalone post, which has no collection.
func (p *AnonymousPost) AbsoluteImages(base string) []string {
	return absoluteImageURLs(p.Content, p.Images, base)
}
