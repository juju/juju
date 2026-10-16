// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

type sshServerPort struct {
	Port int `db:"port"`
}

type controllerSSHHostKey struct {
	ID              string `db:"id"`
	AlgorithmTypeID int    `db:"algorithm_type_id"`
	SSHKey          string `db:"ssh_key"`
	PublicKey       []byte `db:"public_key"`
}

type controllerSSHHostKeyID struct {
	ID string `db:"id"`
}

type userName struct {
	Name string `db:"name"`
}

type userPublicSSHKey struct {
	Comment     string `db:"comment"`
	Fingerprint string `db:"fingerprint"`
	PublicKey   string `db:"public_key"`
}

type entityName struct {
	Name string `db:"name"`
}

type modelUUIDValue struct {
	UUID string `db:"uuid"`
}

type publicKeyFingerprint struct {
	Fingerprint string `db:"fingerprint"`
}

type matchedPublicKeyFingerprint struct {
	Fingerprint string `db:"fingerprint"`
}
