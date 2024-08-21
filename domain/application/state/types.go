// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/life"
)

// These structs represent the persistent block device entity schema in the database.

type KeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// applicationID is used to get the ID (and life) of an application.
type applicationID struct {
	ID     string    `db:"uuid"`
	LifeID life.Life `db:"life_id"`
}

// applicationName is used to get the name of an application.
type applicationName struct {
	Name string `db:"name"`
}

type applicationDetails struct {
	ApplicationID string    `db:"uuid"`
	Name          string    `db:"name"`
	CharmID       string    `db:"charm_uuid"`
	LifeID        life.Life `db:"life_id"`
}

type applicationChannel struct {
	ApplicationID string `db:"application_uuid"`
	Track         string `db:"track"`
	Risk          string `db:"risk"`
	Branch        string `db:"branch"`
}

type applicationScale struct {
	ApplicationID string `db:"application_uuid"`
	Scaling       bool   `db:"scaling"`
	Scale         int    `db:"scale"`
	ScaleTarget   int    `db:"scale_target"`
}

func (as applicationScale) toScaleState() application.ScaleState {
	return application.ScaleState{
		Scaling:     as.Scaling,
		Scale:       as.Scale,
		ScaleTarget: as.ScaleTarget,
	}
}

type unitDetails struct {
	UnitID                  string    `db:"uuid"`
	NetNodeID               string    `db:"net_node_uuid"`
	Name                    string    `db:"name"`
	ApplicationID           string    `db:"application_uuid"`
	LifeID                  life.Life `db:"life_id"`
	PasswordHash            string    `db:"password_hash"`
	PasswordHashAlgorithmID int       `db:"password_hash_algorithm_id"`
}

type coreUnit struct {
	ID     string    `db:"uuid"`
	Name   string    `db:"name"`
	LifeID life.Life `db:"life_id"`
}

type unitCount struct {
	UnitLifeID        life.Life `db:"unit_life_id"`
	ApplicationLifeID life.Life `db:"app_life_id"`
	Count             int       `db:"count"`
}

type cloudContainer struct {
	NetNodeID  string `db:"net_node_uuid"`
	ProviderID string `db:"provider_id"`
}

type cloudService struct {
	ApplicationID string `db:"application_uuid"`
	ProviderID    string `db:"provider_id"`
}

// These structs represent the persistent charm schema in the database.

// charmID represents a single charm row from the charm table, that only
// contains the charm ID.
type charmID struct {
	UUID string `db:"uuid"`
}

type charmUUID struct {
	UUID string `db:"charm_uuid"`
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

// setCharmHash is used to set the hash of a charm.
type setCharmHash struct {
	CharmUUID  string `db:"charm_uuid"`
	HashKindID int    `db:"hash_kind_id"`
	Hash       string `db:"hash"`
}

// setCharmSourceRevisionVersion is used to set the source, revision and
// version of a charm.
type setCharmSourceRevisionVersion struct {
	CharmUUID string `db:"charm_uuid"`
	SourceID  int    `db:"source_id"`
	Revision  int    `db:"revision"`
	Version   string `db:"version"`
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

// setCharmMetadata is used to set the metadata of a charm.
// This includes the setting of the LXD profile.
type setCharmMetadata struct {
	UUID           string `db:"uuid"`
	Name           string `db:"name"`
	Summary        string `db:"summary"`
	Description    string `db:"description"`
	Subordinate    bool   `db:"subordinate"`
	MinJujuVersion string `db:"min_juju_version"`
	Assumes        []byte `db:"assumes"`
	RunAsID        int    `db:"run_as_id"`
	LXDProfile     []byte `db:"lxd_profile"`
	ArchivePath    string `db:"archive_path"`
}

// charmTag is used to get the tags of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmTag struct {
	CharmUUID string `db:"charm_uuid"`
	Tag       string `db:"value"`
}

// setCharmTag is used to set the tags of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmTag struct {
	CharmUUID string `db:"charm_uuid"`
	Tag       string `db:"value"`
	Index     int    `db:"array_index"`
}

// charmCategory is used to get the categories of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmCategory struct {
	CharmUUID string `db:"charm_uuid"`
	Category  string `db:"value"`
}

// setCharmCategory is used to set the categories of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmCategory struct {
	CharmUUID string `db:"charm_uuid"`
	Category  string `db:"value"`
	Index     int    `db:"array_index"`
}

// charmTerm is used to get the terms of a charm.
// This is a row based struct that is normalised form of an array of strings.
type charmTerm struct {
	CharmUUID string `db:"charm_uuid"`
	Term      string `db:"value"`
}

// setCharmTerm is used to set the terms of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmTerm struct {
	CharmUUID string `db:"charm_uuid"`
	Term      string `db:"value"`
	Index     int    `db:"array_index"`
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

// setCharmRelation is used to set the relations of a charm.
type setCharmRelation struct {
	CharmUUID string `db:"charm_uuid"`
	KindID    int    `db:"kind_id"`
	Key       string `db:"key"`
	Name      string `db:"name"`
	RoleID    int    `db:"role_id"`
	Interface string `db:"interface"`
	Optional  bool   `db:"optional"`
	Capacity  int    `db:"capacity"`
	ScopeID   int    `db:"scope_id"`
}

// charmExtraBinding is used to get the extra bindings of a charm.
type charmExtraBinding struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Name      string `db:"name"`
}

// setCharmExtraBinding is used to set the extra bindings of a charm.
type setCharmExtraBinding struct {
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

// setCharmStorage is used to set the storage of a charm.
type setCharmStorage struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	Description string `db:"description"`
	KindID      int    `db:"storage_kind_id"`
	Shared      bool   `db:"shared"`
	ReadOnly    bool   `db:"read_only"`
	CountMin    int    `db:"count_min"`
	CountMax    int    `db:"count_max"`
	MinimumSize uint64 `db:"minimum_size_mib"`
	Location    string `db:"location"`
}

// setCharmStorageProperty is used to set the storage property of a charm.
type setCharmStorageProperty struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"charm_storage_key"`
	Index     int    `db:"array_index"`
	Value     string `db:"value"`
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

// setCharmDevice is used to set the devices of a charm.
type setCharmDevice struct {
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

// setCharmPayload is used to set the payload of a charm.
type setCharmPayload struct {
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

// setCharmResource is used to set the resources of a charm.
type setCharmResource struct {
	CharmUUID   string `db:"charm_uuid"`
	Key         string `db:"key"`
	Name        string `db:"name"`
	KindID      int    `db:"kind_id"`
	Path        string `db:"path"`
	Description string `db:"description"`
}

// charmContainer is used to get the containers of a charm.
// This is a row based struct that is normalised form of an array of strings
// for the storage and location field.
type charmContainer struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Resource  string `db:"resource"`
	Uid       int    `db:"uid"`
	Gid       int    `db:"gid"`
	Storage   string `db:"storage"`
	Location  string `db:"location"`
}

// setCharmContainer is used to set the containers of a charm.
type setCharmContainer struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"key"`
	Resource  string `db:"resource"`
	Uid       int    `db:"uid"`
	Gid       int    `db:"gid"`
}

// setCharmMount is used to set the mounts of a charm.
// This includes the setting of the index.
// This is a row based struct that is normalised form of an array of strings.
type setCharmMount struct {
	CharmUUID string `db:"charm_uuid"`
	Key       string `db:"charm_container_key"`
	Index     int    `db:"array_index"`
	Storage   string `db:"storage"`
	Location  string `db:"location"`
}

// charmManifest is used to get the manifest of a charm.
// This is a row based struct that is normalised form of an array of strings
// for the all the fields.
type charmManifest struct {
	CharmUUID    string `db:"charm_uuid"`
	Index        int    `db:"array_index"`
	NestedIndex  int    `db:"nested_array_index"`
	Track        string `db:"track"`
	Risk         string `db:"risk"`
	Branch       string `db:"branch"`
	OS           string `db:"os"`
	Architecture string `db:"architecture"`
}

// setCharmManifest is used to set the manifest of a charm.
// This includes the setting of the index.
type setCharmManifest struct {
	CharmUUID      string `db:"charm_uuid"`
	Index          int    `db:"array_index"`
	NestedIndex    int    `db:"nested_array_index"`
	Track          string `db:"track"`
	Risk           string `db:"risk"`
	Branch         string `db:"branch"`
	OSID           int    `db:"os_id"`
	ArchitectureID int    `db:"architecture_id"`
}

// charmLXDProfile is used to get the LXD profile of a charm.
type charmLXDProfile struct {
	UUID       string `db:"uuid"`
	LXDProfile []byte `db:"lxd_profile"`
}

// charmConfig is used to get the config of a charm.
// This is a row based struct that is normalised form of a map of config.
type charmConfig struct {
	CharmUUID    string  `db:"charm_uuid"`
	Key          string  `db:"key"`
	Type         string  `db:"type"`
	DefaultValue *string `db:"default_value"`
	Description  string  `db:"description"`
}

// setCharmConfig is used to set the config of a charm.
type setCharmConfig struct {
	CharmUUID    string  `db:"charm_uuid"`
	Key          string  `db:"key"`
	TypeID       int     `db:"type_id"`
	DefaultValue *string `db:"default_value"`
	Description  string  `db:"description"`
}

// charmAction is used to get the actions of a charm.
// This is a row based struct that is normalised form of a map of actions.
type charmAction struct {
	CharmUUID      string `db:"charm_uuid"`
	Key            string `db:"key"`
	Description    string `db:"description"`
	Parallel       bool   `db:"parallel"`
	ExecutionGroup string `db:"execution_group"`
	Params         []byte `db:"params"`
}

// setCharmAction is used to set the actions of a charm.
type setCharmAction struct {
	CharmUUID      string `db:"charm_uuid"`
	Key            string `db:"key"`
	Description    string `db:"description"`
	Parallel       bool   `db:"parallel"`
	ExecutionGroup string `db:"execution_group"`
	Params         []byte `db:"params"`
}

// charmArchivePath is used to get the archive path of a charm.
type charmArchivePath struct {
	ArchivePath string `db:"archive_path"`
}

// charmOrigin is used to get the origin of a charm.
type charmOrigin struct {
	CharmID  string `db:"charm_uuid"`
	Source   string `db:"source"`
	Revision int    `db:"revision"`
}

// setCharmOrigin is used to set the origin of a charm.
type setCharmOrigin struct {
	CharmID  string `db:"charm_uuid"`
	SourceID int    `db:"source_id"`
	Revision int    `db:"revision"`
}

type charmPlatform struct {
	CharmID        string                   `db:"charm_uuid"`
	OSTypeID       application.OSType       `db:"os_id"`
	Channel        string                   `db:"channel"`
	ArchitectureID application.Architecture `db:"architecture_id"`
}

type countResult struct {
	Count int `db:"count"`
}
