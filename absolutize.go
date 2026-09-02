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

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// urlAttrs are the attributes whose values are a single URL.
var urlAttrs = map[string]bool{
	"href":   true,
	"src":    true,
	"poster": true,
}

// srcsetAttrs are the attributes whose values are a comma-separated candidate
// list, each candidate being a URL followed by an optional descriptor.
var srcsetAttrs = map[string]bool{
	"srcset": true,
}

// absolutizeHTML resolves every relative URL in content against base.
//
// Rendered post HTML keeps its URLs relative so that the web view stays
// host-independent, but syndicated copies leave this instance: a feed reader
// or a remote fediverse server resolves what it receives against its own
// address, not ours, so a relative URL there points at the wrong host. Callers
// apply this at those boundaries only — never to the HTML served to the web.
//
// Resolution is idempotent: an already-absolute URL resolves to itself, as do
// scheme-carrying values like data: and mailto:. A protocol-relative URL picks
// up base's scheme, which is the intended result. Content is returned unchanged
// if base is unusable or the content cannot be parsed, since dropping post
// content would be worse than leaving a URL relative.
func absolutizeHTML(content, base string) string {
	if content == "" {
		return content
	}

	b, err := url.Parse(base)
	if err != nil || !b.IsAbs() {
		return content
	}

	body := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(content), body)
	if err != nil {
		return content
	}

	var out strings.Builder
	for _, n := range nodes {
		absolutizeNode(n, b)
		if err := xhtml.Render(&out, n); err != nil {
			return content
		}
	}
	return out.String()
}

func absolutizeNode(n *xhtml.Node, base *url.URL) {
	if n.Type == xhtml.ElementNode {
		for i, attr := range n.Attr {
			switch {
			case urlAttrs[attr.Key]:
				n.Attr[i].Val = absolutizeURL(attr.Val, base)
			case srcsetAttrs[attr.Key]:
				n.Attr[i].Val = absolutizeSrcset(attr.Val, base)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		absolutizeNode(c, base)
	}
}

func absolutizeURL(v string, base *url.URL) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return v
	}
	ref, err := url.Parse(trimmed)
	if err != nil {
		return v
	}
	return base.ResolveReference(ref).String()
}

// absolutizeSrcset resolves each candidate in a srcset, preserving descriptors.
func absolutizeSrcset(v string, base *url.URL) string {
	candidates := strings.Split(v, ",")
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		u, descriptor, found := strings.Cut(candidate, " ")
		resolved := absolutizeURL(u, base)
		if found {
			resolved += " " + strings.TrimSpace(descriptor)
		}
		out = append(out, resolved)
	}
	return strings.Join(out, ", ")
}

// syndicatedContent returns a post's rendered HTML with every relative URL
// resolved against permalink — the post's own address — ready to leave this
// instance in a feed item or an ActivityPub object.
//
// The permalink, rather than the site host, is the base so that both
// root-relative URLs ("/uploads/pic.png") and relative ones ("pic.png")
// resolve the way a browser on the post page would resolve them.
func syndicatedContent(p *PublicPost, permalink string) string {
	return absolutizeHTML(string(p.HTMLContent), permalink)
}
