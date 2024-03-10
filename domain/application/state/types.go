// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent block device entity schema in the database.

type KeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}
