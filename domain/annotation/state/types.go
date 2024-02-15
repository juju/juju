// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// Annotation represents an annotation in the state layer that we read/write to/from DB.
type Annotation struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}
