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

import "strings"

// parseVerificationLinks splits a stored verification_link attribute into
// its individual links. The attribute holds newline-separated URLs; a
// value saved before multiple links were supported contains no newline and
// parses as a single-element slice. Blank entries are dropped and
// duplicates removed, keeping the first occurrence, since link order is
// meaningful: the first link is the blog's canonical identity.
func parseVerificationLinks(stored string) []string {
	links := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(stored, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		links = append(links, l)
	}
	return links
}

// serializeVerificationLinks joins links into the newline-separated form
// stored in the verification_link attribute, applying the same trimming
// and deduplication as parseVerificationLinks.
func serializeVerificationLinks(links []string) string {
	return strings.Join(parseVerificationLinks(strings.Join(links, "\n")), "\n")
}
