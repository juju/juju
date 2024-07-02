// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent charm schema in the database.

// charmID represents a single charm row from the charm table, that only
// contains the charm id.
type charmID struct {
	UUID string `db:"uuid"`
}

// charmName is used to pass the name to the query.
type charmName struct {
	Name string `db:"name"`
}

// charmNameRevision is used to pass the name and revision to the query.
type charmNameRevision struct {
	Name     string `db:"name"`
	Revision int    `db:"revision"`
}

// charmAvailable is used to get the available status of a charm.
type charmAvailable struct {
	Available bool `db:"available"`
}

// charmSubordinate is used to get the subordinate status of a charm.
type charmSubordinate struct {
	Subordinate bool `db:"subordinate"`
}

type charm struct {
	UUID           string `db:"uuid"`
	Name           string `db:"name"`
	Summary        string `db:"summary"`
	Description    string `db:"description"`
	Subordinate    bool   `db:"subordinate"`
	MinJujuVersion string `db:"min_juju_version"`
	RunAsID        string `db:"run_as_id"`
	Assumes        []byte `db:"assumes"`
	LXDProfile     []byte `db:"lxd_profile"`
}

type charmState struct {
	CharmUUID string `db:"charm_uuid"`
	Available bool   `db:"available"`
}
