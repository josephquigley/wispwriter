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
	"strings"
	"testing"
	"time"
)

// imageURLPattern is how a post body is searched for the uploads it
// references, and imagePath is how those uploads are named. Nothing links the
// two: the pattern restates imageSlug's rules as a regexp, and they agree only
// because slug.Make lowercases, strips dots and collapses everything else to
// hyphens.
//
// The failure mode if they ever drift is silent and destructive. An upload the
// pattern cannot match looks unreferenced to attachPostImages, so editing the
// post detaches it, and removeImageIfUnreferenced then deletes the row and the
// file — losing an image that is still in the post.
//
// So: whatever imagePath produces, imageURLPattern must match, and the path it
// captures must be exactly the stored path — that captured value is what
// GetPostImageByPath looks up.
func TestStoredImagePathsAreMatchedByTheReferencePattern(t *testing.T) {
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	filenames := []string{
		"pic.png",
		"Screenshot 2026-04-22 at 17.05.29.png", // spaces, capitals, dots in the stem
		"100_3377-1.JPG",                        // underscore, capitals in the extension
		"P1200105-full.jpg",                     // leading capital
		"BF05C530-C229-4B4D-A14A-6042810E.jpeg", // long hyphenated capitals
		"ÜBER Ähnlich.png",                      // non-ASCII
		"图片.png",                                // non-Latin script
		"archive.tar.gz.png",                    // several dots
		"..hidden..png",                         // leading dots
		"   spaced out   .png",                  // surrounding whitespace
		"%encoded%20name.png",                   // percent signs
		"a+b&c=d.png",                           // URL-significant characters
		strings.Repeat("long", 100) + ".png",    // beyond imageSlugMaxLen
		".png",                                  // no stem at all
	}

	for _, name := range filenames {
		t.Run(name, func(t *testing.T) {
			for _, n := range []int{1, 2} {
				path := imagePath(name, "png", at, n)
				url := "/" + uploadsDir + "/" + path

				m := imageURLPattern.FindStringSubmatch(url)
				if m == nil {
					t.Fatalf("imageURLPattern does not match a path imagePath produced: %s", url)
				}
				if m[1] != path {
					t.Errorf("pattern captured %q, but the stored path is %q", m[1], path)
				}
			}
		})
	}
}

// The same guarantee has to hold when the URL is absolute, which is how post
// bodies reference uploads after a migration from another blog engine, and how
// syndicated copies always reference them.
func TestStoredImagePathsMatchInsideAbsoluteURLs(t *testing.T) {
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	path := imagePath("Screenshot 2026-04-22 at 17.05.29.png", "png", at, 1)

	url := "https://quigs.blog/" + uploadsDir + "/" + path

	m := imageURLPattern.FindStringSubmatch(url)
	if m == nil {
		t.Fatalf("imageURLPattern does not match an absolute upload URL: %s", url)
	}
	if m[1] != path {
		t.Errorf("pattern captured %q, but the stored path is %q", m[1], path)
	}
}
