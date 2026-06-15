// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import "database/sql"

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
	MachineUUID string `db:"machine_uuid"`
	SSHKey      string `db:"ssh_key"`
}

type unitVirtualSSHHostKey struct {
	UnitUUID string `db:"unit_uuid"`
	SSHKey   string `db:"ssh_key"`
}

type unitMachine struct {
	MachineName sql.NullString `db:"machine_name"`
}
