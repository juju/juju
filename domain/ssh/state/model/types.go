// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"
	"time"
)

type entityName struct {
	Name string `db:"name"`
}

type entityUUID struct {
	UUID string `db:"uuid"`
}

type sshPrivateKey struct {
	SSHKey string `db:"ssh_key"`
}

type machineVirtualSSHHostKey struct {
	MachineUUID     string `db:"machine_uuid"`
	AlgorithmTypeID int    `db:"algorithm_type_id"`
	SSHKey          string `db:"ssh_key"`
}

type unitVirtualSSHHostKey struct {
	UnitUUID        string `db:"unit_uuid"`
	AlgorithmTypeID int    `db:"algorithm_type_id"`
	SSHKey          string `db:"ssh_key"`
}

type unitMachine struct {
	MachineName sql.NullString `db:"machine_name"`
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
	ExpiresAt           time.Time `db:"expires_at"`
	Username            string    `db:"username"`
	Password            string    `db:"password"`
	ControllerAddresses string    `db:"controller_addresses"`
	UnitPort            int       `db:"unit_port"`
	EphemeralPublicKey  []byte    `db:"ephemeral_public_key"`
}

type sshConnRequestRecord struct {
	TunnelID            string    `db:"tunnel_id"`
	MachineID           string    `db:"machine_id"`
	ExpiresAt           time.Time `db:"expires_at"`
	Username            string    `db:"username"`
	Password            string    `db:"password"`
	ControllerAddresses string    `db:"controller_addresses"`
	UnitPort            int       `db:"unit_port"`
	EphemeralPublicKey  []byte    `db:"ephemeral_public_key"`
}
