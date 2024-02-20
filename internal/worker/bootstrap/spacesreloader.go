// Copyright 2024 Canonical Ltd. Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
)

// ensureInitialModelSpaces will create the loaded spaces (and subnets) from
// the provider (if any) into the initial model which is created during
// bootstrap.
//
// Since at bootstrap we have spaces that are present in the provider but not
// assigned to any juju model, and we also know that the spaces have already
// been loaded from the provider using ReloadSpaces(), we can now simply copy
// those spaces that are assigned to the controller model into the initial
// model.
// The only spaces that are not copied are the HASpace and the management
// space.
func (w *bootstrapWorker) ensureInitialModelSpaces(
	ctx context.Context,
	initialModelUUID string,
	controllerConfig controller.Config,
) error {
	spacesToCopy := network.SpaceInfos{}
	// We can use the space service in the worker config, which corresponds
	// to the controller model, to retrieve the existent spaces.
	allSpaces, err := w.cfg.SpaceService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, space := range allSpaces {
		if space.ID != controllerConfig.JujuHASpace() &&
			space.ID != controllerConfig.JujuManagementSpace() &&
			space.ID != network.AlphaSpaceId {
			spacesToCopy = append(spacesToCopy, space)
		}
	}
	if len(spacesToCopy) > 0 {
		// At this point we must create a service factory for the
		// initial model from which we can create the space and subnet
		// services for the initial model that we'll use for inserting
		// the spaces and subnets.
		initialModelServiceFactory := w.cfg.ServiceFactoryGetter.FactoryForModel(initialModelUUID)
		initialModelSpaceService := w.cfg.SpaceServiceGetter(initialModelServiceFactory)
		initialModelSubnetService := w.cfg.SubnetServiceGetter(initialModelServiceFactory)

		for _, space := range spacesToCopy {
			// We first retrieve all the subnets from controller
			// model, and then we insert them all into the initial
			// model database.
			var subnetIDs []string
			for _, subnet := range space.Subnets {
				// We must clear its ID (which is the UUID
				// corresponding to the subnet as stored in the
				// controller model database).
				subnet.ID = ""
				subnet.SpaceID = ""
				subnetID, err := initialModelSubnetService.AddSubnet(ctx, subnet)
				if err != nil {
					return errors.Trace(err)
				}
				subnetIDs = append(subnetIDs, subnetID.String())
			}
			// We insert the new space with the IDs of the subnets
			// we have just copied to the initial model.
			insertedSpaceID, err := initialModelSpaceService.AddSpace(ctx, string(space.Name), space.ProviderId, subnetIDs)
			if err != nil {
				return errors.Trace(err)
			}
			for _, subnetID := range subnetIDs {
				err := initialModelSubnetService.UpdateSubnet(ctx, subnetID, insertedSpaceID.String())
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
	return nil
}
