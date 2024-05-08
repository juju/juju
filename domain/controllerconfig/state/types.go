// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type dbKeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

type dbController struct {
	UUID string `db:"uuid"`
}
