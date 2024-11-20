// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// Annotation represents an annotation in the state layer that we read/write
// to/from DB.
type Annotation struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// annotationUUID represents the struct to be used for the UUID column of
// annotation tables within the SQLair statements in the annotation domain.
type annotationUUID struct {
	UUID string `db:"uuid"`
}

type charmArgs struct {
	Name     string `db:"name"`
	Revision int    `db:"revision"`
}
