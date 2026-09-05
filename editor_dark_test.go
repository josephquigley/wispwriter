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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDarkModeRestylesTheAceEditor guards the Custom CSS box on a blog's
// settings page. Ace replaces the textarea it is given with a <pre>, so the
// dark block's `pre, code` rule paints the widget's background dark while
// Ace's own light theme keeps painting the gutter and the syntax colours for
// a white page. Half a widget is worse than either whole one: the result was
// a light gutter beside a dark editor, and comment green and string blue at
// roughly 2:1 against the new background.
//
// The theme cannot be swapped instead, because the only Ace theme this
// repository ships is chrome, and a dark one would be a new vendored
// dependency. So the palette lives here, and these are the parts of the
// widget that have to be named for it to be consistent.
func TestDarkModeRestylesTheAceEditor(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("less", "dark.less"))
	if err != nil {
		t.Fatalf("read dark.less: %v", err)
	}
	src := string(b)

	parts := []struct {
		selector string
		why      string
	}{
		{".ace_editor", "the widget itself, which core.less gives a light border"},
		{".ace_gutter", "the annotation column, which Ace's theme paints #ebebeb"},
		{".ace_cursor", "the caret, which is black against the dark background"},
		{".ace_comment", "comments, Ace's darkest syntax colour"},
		{".ace_string", "strings, which Ace paints #1A1AA6 for a white page"},
		{".ace_keyword", "selectors and at-rules"},
		{".ace_tooltip", "the annotation popup the gutter icons open"},
	}
	for _, p := range parts {
		if !strings.Contains(src, p.selector) {
			t.Errorf("dark.less does not style %s (%s), so it stays light inside a dark editor", p.selector, p.why)
		}
	}
}
