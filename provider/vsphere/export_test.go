// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/environs/imagemetadata"
	"github.com/juju/juju/v2/provider/vsphere/internal/vsphereclient"
)

// NewVMTemplateManager returns a new vmTemplateManager. This function
// is useful for testing.
func NewVMTemplateManager(
	imgMeta []*imagemetadata.ImageMetadata,
	env environs.Environ, client Client,
	azRef types.ManagedObjectReference,
	datastore *object.Datastore,
	statusUpdateArgs vsphereclient.StatusUpdateParams,
	vmFolder, controllerUUID string) vmTemplateManager {

	return vmTemplateManager{
		imageMetadata:    imgMeta,
		env:              env,
		client:           client,
		azPoolRef:        azRef,
		datastore:        datastore,
		statusUpdateArgs: statusUpdateArgs,

		vmFolder:       vmFolder,
		controllerUUID: controllerUUID,
	}
}
