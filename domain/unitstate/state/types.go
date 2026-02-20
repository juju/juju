// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/network"
)

// unitUUID identifies a unit.
type unitUUID struct {
	// UUID is the universally unique identifier for a unit.
	UUID string `db:"uuid"`
}

// unitName identifies a unit.
type unitName struct {
	// Name uniquely identifies a unit and indicates its application.
	// For example, postgresql/3.
	Name string `db:"name"`
}

// portRange represents a range of ports for a given protocol.
type portRange struct {
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
}

// decode returns the network.PortRange representation of the portRange.
func (pr portRange) decode() network.PortRange {
	return network.PortRange{
		Protocol: pr.Protocol,
		FromPort: pr.FromPort,
		ToPort:   pr.ToPort,
	}
}

// endpoint represents a network endpoint and its UUID.
type endpoint struct {
	UUID     string `db:"uuid"`
	Endpoint string `db:"endpoint"`
}

// endpointName represents a network endpoint's name.
type endpointName struct {
	Endpoint string `db:"endpoint"`
}

// endpoints represents a list of network endpoints.
type endpoints []string

type portRangeUUIDs []string

// endpointPortRangeUUID represents an endpointPortRange with the port range
// UUID.
type endpointPortRangeUUID struct {
	UUID     string `db:"uuid"`
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

// decode returns the network.PortRange representation of the endpointPortRangeUUID.
func (p endpointPortRangeUUID) decode() network.PortRange {
	return network.PortRange{
		Protocol: p.Protocol,
		FromPort: p.FromPort,
		ToPort:   p.ToPort,
	}
}

// protocol represents a network protocol type and its ID in DQLite.
type protocol struct {
	ID   int    `db:"id"`
	Name string `db:"protocol"`
}

// unitPortRange represents a range of ports for a given protocol by id for a
// given unit's endpoint by uuid.
type unitPortRange struct {
	UUID         string `db:"uuid"`
	ProtocolID   int    `db:"protocol_id"`
	FromPort     int    `db:"from_port"`
	ToPort       int    `db:"to_port"`
	RelationUUID string `db:"relation_uuid,omitempty"`
	UnitUUID     string `db:"unit_uuid"`
}

// unitState contains a YAML string representing the
// state for a unit's uniter, storage and secrets.
type unitState struct {
	// UniterState is the units uniter state YAML string.
	UniterState string `db:"uniter_state"`
	// StorageState is the units storage state YAML string.
	StorageState string `db:"storage_state"`
	// SecretState is the units secret state YAML string.
	SecretState string `db:"secret_state"`
}

// unitStateVal is a type for holding a key/value pair that is
// a constituent in unit state for charm and relation.
type unitStateKeyVal[T comparable] struct {
	UUID  string `db:"unit_uuid"`
	Key   T      `db:"key"`
	Value string `db:"value"`
}

type unitCharmStateKeyVal unitStateKeyVal[string]
type unitRelationStateKeyVal unitStateKeyVal[int]

func makeUnitCharmStateKeyVals(unitUUID unitUUID, kv map[string]string) []unitCharmStateKeyVal {
	keyVals := make([]unitCharmStateKeyVal, 0, len(kv))
	for k, v := range kv {
		keyVals = append(keyVals, unitCharmStateKeyVal{
			UUID:  unitUUID.UUID,
			Key:   k,
			Value: v,
		})
	}
	return keyVals
}

func makeUnitRelationStateKeyVals(unitUUID unitUUID, kv map[int]string) []unitRelationStateKeyVal {
	keyVals := make([]unitRelationStateKeyVal, 0, len(kv))
	for k, v := range kv {
		keyVals = append(keyVals, unitRelationStateKeyVal{
			UUID:  unitUUID.UUID,
			Key:   k,
			Value: v,
		})
	}
	return keyVals
}

func makeMapFromCharmUnitStateKeyVals(us []unitCharmStateKeyVal) map[string]string {
	m := map[string]string{}
	for _, kv := range us {
		m[kv.Key] = kv.Value
	}
	return m
}

func makeMapFromRelationUnitStateKeyVals(us []unitRelationStateKeyVal) map[int]string {
	m := map[int]string{}
	for _, kv := range us {
		m[kv.Key] = kv.Value
	}
	return m
}
