// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/core/network"

// protocol represents a network protocol type and its ID in DQLite.
type protocol struct {
	ID   int    `db:"id"`
	Name string `db:"protocol"`
}

// endpointPortRange represents a range of ports for a give protocol for a
// given endpoint.
type endpointPortRange struct {
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

func (epr endpointPortRange) decode() network.PortRange {
	return network.PortRange{
		Protocol: epr.Protocol,
		FromPort: epr.FromPort,
		ToPort:   epr.ToPort,
	}
}

type endpointPortRangeUUID struct {
	UUID     string `db:"uuid"`
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

func (p endpointPortRangeUUID) decode() network.PortRange {
	return network.PortRange{
		Protocol: p.Protocol,
		FromPort: p.FromPort,
		ToPort:   p.ToPort,
	}
}

type portRangeUUIDs []string

// unitPortRange represents a range of ports for a given protocol by id for a
// given unit's endpoint by uuid.
type unitPortRange struct {
	UUID             string `db:"uuid"`
	ProtocolID       int    `db:"protocol_id"`
	FromPort         int    `db:"from_port"`
	ToPort           int    `db:"to_port"`
	UnitEndpointUUID string `db:"unit_endpoint_uuid"`
}

// endpoint represents a network endpoint and its UUID.
type endpoint struct {
	UUID     string `db:"uuid"`
	Endpoint string `db:"endpoint"`
}

// endpoints represents a list of network endpoints.
type endpoints []string

// unitEndpoint represents a unit's endpoint and its UUID.
type unitEndpoint struct {
	UUID     string `db:"uuid"`
	Endpoint string `db:"endpoint"`
	UnitUUID string `db:"unit_uuid"`
}

// unitUUID represents a unit's UUID.
type unitUUID struct {
	UUID string `db:"unit_uuid"`
}
