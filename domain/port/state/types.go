// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/port"
)

// protocol represents a network protocol type and its ID in DQLite.
type protocol struct {
	ID   int    `db:"id"`
	Name string `db:"protocol"`
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

// endpointPortRange represents a range of ports for a give protocol for a
// given endpoint.
type endpointPortRange struct {
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

// decode returns the network.PortRange representation of the endpointPortRange.
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

// decode returns the network.PortRange representation of the endpointPortRangeUUID.
func (p endpointPortRangeUUID) decode() network.PortRange {
	return network.PortRange{
		Protocol: p.Protocol,
		FromPort: p.FromPort,
		ToPort:   p.ToPort,
	}
}

// unitEndpointPortRange represents a range of ports for a given protocol for
// a given unit's endpoint, and unit UUID.
type unitEndpointPortRange struct {
	UnitUUID string `db:"unit_uuid"`
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

func (p unitEndpointPortRange) decodeToUnitEndpointPortRange() port.UnitEndpointPortRange {
	return port.UnitEndpointPortRange{
		UnitUUID:  unit.UUID(p.UnitUUID),
		Endpoint:  p.Endpoint,
		PortRange: p.decodeToPortRange(),
	}
}

func (p unitEndpointPortRange) decodeToPortRange() network.PortRange {
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

// endpointUUID represents an endpoint's UUID.
type endpointUUID struct {
	UUID string `db:"uuid"`
}

// endpointName represents a network endpoint's name.
type endpointName struct {
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

// machineUUID represents a machine's UUID.
type machineUUID struct {
	UUID string `db:"machine_uuid"`
}

// applicationUUID represents an application's UUID.
type applicationUUID struct {
	UUID string `db:"application_uuid"`
}

// name represents a "name" column in the database.
type name struct {
	Name string `db:"name"`
}
