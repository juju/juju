// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// These structs represent the persistent charm schema in the database.

// charmID represents a single charm row from the charm table, that only
// contains the charm ID.
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

// charmIDName is used to get the ID and name of a charm.
type charmIDName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// charmState is used to get the state of a charm.
type charmState struct {
	CharmUUID string `db:"charm_uuid"`
	Available bool   `db:"available"`
}

// charmMetadata is used to get the metadata of a charm.
type charmMetadata struct {
	Name           string `db:"name"`
	Summary        string `db:"summary"`
	Description    string `db:"description"`
	Subordinate    bool   `db:"subordinate"`
	MinJujuVersion string `db:"min_juju_version"`
	Assumes        []byte `db:"assumes"`
	RunAs          string `db:"run_as"`
}

// charmTag is used to get the tags of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmTag struct {
	CharmUUID string `db:"charm_uuid"`
	Tag       string `db:"value"`
}

// charmCategory is used to get the categories of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmCategory struct {
	CharmUUID string `db:"charm_uuid"`
	Category  string `db:"value"`
}

// charmTerm is used to get the terms of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmTerm struct {
	CharmUUID string `db:"charm_uuid"`
	Term      string `db:"value"`
}

// charmRelation is used to get the relations of a charm.
type charmRelation struct {
	CharmUUID string `db:"charm_uuid"`
	Kind      string `db:"kind"`
	Key       string `db:"key"`
	Name      string `db:"name"`
	Role      string `db:"role"`
	Interface string `db:"interface"`
	Optional  bool   `db:"optional"`
	Capacity  int    `db:"capacity"`
	Scope     string `db:"scope"`
}

// charmExtraBinding is used to get the extra bindings of a charm.
type charmExtraBinding struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Name      string `db:"name"`
}

// charmStorage is used to get the storage of a charm.
// This is a row based struct that is normalised form of an array of strings
// for the property field.
type charmStorage struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Kind        string `db:"kind"`
	Shared      bool   `db:"shared"`
	ReadOnly    bool   `db:"read_only"`
	CountMin    int    `db:"count_min"`
	CountMax    int    `db:"count_max"`
	MinimumSize uint64 `db:"minimum_size_mib"`
	Location    string `db:"location"`
	Property    string `db:"property"`
}

// charmDevice is used to get the devices of a charm.
type charmDevice struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Description string `db:"description"`
	DeviceType  string `db:"device_type"`
	CountMin    int64  `db:"count_min"`
	CountMax    int64  `db:"count_max"`
}

// charmPayload is used to get the payload of a charm.
type charmPayload struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Name      string `db:"name"`
	Type      string `db:"type"`
}

// charmResource is used to get the resources of a charm.
type charmResource struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Kind        string `db:"kind"`
	Path        string `db:"path"`
	Description string `db:"description"`
}

type charmContainer struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Resource  string `db:"resource"`
	Uid       int    `db:"uid"`
	Gid       int    `db:"gid"`
	Storage   string `db:"storage"`
	Location  string `db:"location"`
}
