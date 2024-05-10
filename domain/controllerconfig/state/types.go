// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type KeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

type Controller struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type StringSlice []string
