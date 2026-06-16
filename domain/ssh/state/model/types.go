// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "time"

type machineName struct {
	Name string `db:"name"`
}

type machineUUID struct {
	UUID string `db:"uuid"`
}

type tunnelID struct {
	TunnelID string `db:"tunnel_id"`
}

type expiryTime struct {
	ExpiresAt time.Time `db:"expires_at"`
}

type sshConnRequestInsert struct {
	TunnelID            string    `db:"tunnel_id"`
	MachineUUID         string    `db:"machine_uuid"`
	Expires             time.Time `db:"expires_at"`
	Username            string    `db:"username"`
	Password            string    `db:"password"`
	ControllerAddresses string    `db:"controller_addresses"`
	UnitPort            int       `db:"unit_port"`
	EphemeralPublicKey  []byte    `db:"ephemeral_public_key"`
}

type sshConnRequestRecord struct {
	TunnelID            string    `db:"tunnel_id"`
	MachineID           string    `db:"machine_id"`
	Expires             time.Time `db:"expires_at"`
	Username            string    `db:"username"`
	Password            string    `db:"password"`
	ControllerAddresses string    `db:"controller_addresses"`
	UnitPort            int       `db:"unit_port"`
	EphemeralPublicKey  []byte    `db:"ephemeral_public_key"`
}
