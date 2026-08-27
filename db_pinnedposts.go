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
	"github.com/writeas/web-core/log"
)

// NormalizePinnedPositions rewrites a collection's pinned_position values
// to a dense 1..n sequence, preserving the current order and breaking ties
// by creation time.
//
// Positions drift because pinning only ever appends
// (GetLastPinnedPostPos + 1) and unpinning sets the position to NULL
// without renumbering, so an established blog accumulates gaps, and
// concurrent pins can produce duplicates. Reordering needs a dense
// sequence to be able to swap adjacent entries, so this runs before every
// read and after every change. It is idempotent.
func (db *datastore) NormalizePinnedPositions(collID int64) error {
	rows, err := db.Query("SELECT id FROM posts WHERE collection_id = ? AND pinned_position IS NOT NULL ORDER BY pinned_position ASC, created ASC", collID)
	if err != nil {
		log.Error("Failed selecting pinned posts to normalize: %v", err)
		return err
	}

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			log.Error("Failed scanning pinned post to normalize: %v", err)
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Error("Failed reading pinned posts to normalize: %v", err)
		return err
	}
	// The cursor is closed before the transaction opens: holding a read
	// cursor open across writes on the same connection deadlocks on SQLite.
	rows.Close()

	if len(ids) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		log.Error("Failed starting pinned position normalization: %v", err)
		return err
	}
	for i, id := range ids {
		if _, err := tx.Exec("UPDATE posts SET pinned_position = ? WHERE id = ?", int64(i+1), id); err != nil {
			tx.Rollback()
			log.Error("Failed normalizing pinned position: %v", err)
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		log.Error("Failed committing pinned position normalization: %v", err)
		return err
	}
	return nil
}
