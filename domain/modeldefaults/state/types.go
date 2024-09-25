// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type cloudDefaults struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

type cloudType struct {
	Type string `db:"type"`
}

type modelMetadata struct {
	Name      string `db:"name"`
	CloudType string `db:"type"`
}

type modelUUID struct {
	UUID string `db:"uuid"`
}
