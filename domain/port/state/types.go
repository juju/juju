// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/errors"

const (
	UnitEndpointNotFound = errors.ConstError("unit endpoint not found")
)

type protocol struct {
	ID   int    `db:"id"`
	Name string `db:"protocol"`
}

type portRange struct {
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
}

type endpointPortRange struct {
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

type unitPortRange struct {
	ProtocolID       int    `db:"protocol_id"`
	FromPort         int    `db:"from_port"`
	ToPort           int    `db:"to_port"`
	UnitEndpointUUID string `db:"unit_endpoint_uuid"`
}

type unitEndpointUUID struct {
	UUID string `db:"uuid"`
}

type unitEndpointUUIDs []string

type unitUUIDEndpoint struct {
	UUID     string `db:"unit_uuid"`
	Endpoint string `db:"endpoint"`
}

type unitEndpoint struct {
	UUID     string `db:"uuid"`
	Endpoint string `db:"endpoint"`
	UnitUUID string `db:"unit_uuid"`
}

type unitUUID struct {
	UUID string `db:"unit_uuid"`
}
