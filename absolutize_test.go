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

import "testing"

func TestAbsolutizeHTML(t *testing.T) {
	const base = "https://quigs.blog/rel-img-test"

	tests := []struct {
		name   string
		in     string
		result string
	}{
		{"empty", "", ""},
		{"no urls", "<p>Hello</p>", "<p>Hello</p>"},
		{"root-relative img", `<p><img src="/uploads/2026/09/02/pic.png" alt="pic"></p>`, `<p><img src="https://quigs.blog/uploads/2026/09/02/pic.png" alt="pic"/></p>`},
		{"bare relative img", `<img src="pic.png">`, `<img src="https://quigs.blog/pic.png"/>`},
		{"absolute img untouched", `<img src="https://example.com/a.png">`, `<img src="https://example.com/a.png"/>`},
		{"data uri untouched", `<img src="data:image/png;base64,AAAA">`, `<img src="data:image/png;base64,AAAA"/>`},
		{"mailto untouched", `<a href="mailto:x@example.com">m</a>`, `<a href="mailto:x@example.com">m</a>`},
		{"protocol-relative gains scheme", `<img src="//cdn.example.com/a.png">`, `<img src="https://cdn.example.com/a.png"/>`},
		{"root-relative link", `<a href="/some-post">x</a>`, `<a href="https://quigs.blog/some-post">x</a>`},
		{"srcset candidates", `<img src="/a.png" srcset="/a.png 1x, /a2.png 2x">`, `<img src="https://quigs.blog/a.png" srcset="https://quigs.blog/a.png 1x, https://quigs.blog/a2.png 2x"/>`},
		{"video poster and source", `<video poster="/p.jpg"><source src="/v.mp4"></video>`, `<video poster="https://quigs.blog/p.jpg"><source src="https://quigs.blog/v.mp4"/></video>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := absolutizeHTML(test.in, base)
			if res != test.result {
				t.Errorf("%s: wanted %s, got %s", test.name, test.result, res)
			}
		})
	}
}

func TestAbsolutizeHTMLIsIdempotent(t *testing.T) {
	const base = "https://quigs.blog/rel-img-test"
	in := `<p><img src="/uploads/pic.png"></p><a href="/some-post">x</a>`

	once := absolutizeHTML(in, base)
	twice := absolutizeHTML(once, base)
	if once != twice {
		t.Errorf("not idempotent:\n once: %s\ntwice: %s", once, twice)
	}
}

func TestAbsolutizeHTMLWithUnusableBaseReturnsInput(t *testing.T) {
	in := `<img src="/uploads/pic.png">`
	for _, base := range []string{"", "://nope"} {
		if res := absolutizeHTML(in, base); res != in {
			t.Errorf("base %q: wanted input unchanged, got %s", base, res)
		}
	}
}
