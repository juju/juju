// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storageprovisioning"
)

// ImportStorageInstanceArgs represents data to import a storage instance
// and its owner.
type ImportStorageInstanceArgs struct {
	UUID             string
	Life             int
	PoolName         string
	RequestedSizeMiB uint64
	StorageName      string
	StorageKind      string
	StorageID        string
	UnitName         string
}

// ImportFilesystemIAASArgs represents data to import a filesystem.
type ImportFilesystemIAASArgs struct {
	UUID                string
	ID                  string
	Life                life.Life
	SizeInMiB           uint64
	ProviderID          string
	StorageInstanceUUID string
	Scope               storageprovisioning.ProvisionScope
}

// ImportFilesystemAttachmentIAASArgs represents data to import filesystem attachments.
type ImportFilesystemAttachmentIAASArgs struct {
	UUID           string
	FilesystemUUID string
	NetNodeUUID    string
	Scope          storageprovisioning.ProvisionScope
	Life           life.Life
	MountPoint     string
	ReadOnly       bool
}
