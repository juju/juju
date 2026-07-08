// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package types

import (
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/storage"
)

// ProvisioningInfo holds the info needed to provision a CAAS operator.
type ProvisioningInfo struct {
	Version              semversion.Number
	APIAddresses         []string
	CACert               string
	Tags                 map[string]string
	Constraints          constraints.Value
	Devices              []devices.KubernetesDeviceParams
	Base                 base.Base
	ImageDetails         resource.DockerImageDetails
	CharmModifiedVersion int
	Trust                bool
	Scale                int
}

// FilesystemProvisioningInfo holds the filesystem info needed to provision
// a CAAS operator for an application.
type FilesystemProvisioningInfo struct {
	Filesystems               []storage.KubernetesFilesystemParams
	FilesystemUnitAttachments map[string][]storage.KubernetesFilesystemUnitAttachmentParams
}
