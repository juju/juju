// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/objectstore"
	objectstoretesting "github.com/juju/juju/internal/objectstore/testing"
	"github.com/juju/juju/state"
)

// NewObjectStore creates a new object store for testing.
// This uses the memory metadata service.
func NewObjectStore(c *tc.C, modelUUID string) coreobjectstore.ObjectStore {
	return NewObjectStoreWithMetadataService(c, modelUUID, objectstoretesting.MemoryMetadataService())
}

// NewObjectStoreWithMetadataService creates a new object store for testing.
func NewObjectStoreWithMetadataService(c *tc.C, modelUUID string, metadataService objectstore.MetadataService) coreobjectstore.ObjectStore {
	store, err := objectstore.ObjectStoreFactory(
		context.Background(),
		objectstore.DefaultBackendType(),
		modelUUID,
		objectstore.WithRootDir(c.MkDir()),
		objectstore.WithLogger(loggertesting.WrapCheckLog(c)),

		// TODO (stickupkid): Swap this over to the real metadata service
		// when all facades are moved across.
		objectstore.WithMetadataService(metadataService),
		objectstore.WithClaimer(objectstoretesting.MemoryClaimer()),
	)
	c.Assert(err, jc.ErrorIsNil)
	return store
}

// AddControllerMachine adds a "controller" machine to the state so
// that State.Addresses and State.APIAddresses will work. It returns the
// added machine. The addresses that those methods will return bear no
// relation to the addresses actually used by the state and API servers.
// It returns the addresses that will be returned by the State.Addresses
// and State.APIAddresses methods, which will not bear any relation to
// the be the addresses used by the controllers.
func AddControllerMachine(
	c *tc.C,
	st *state.State,
	controllerConfig controller.Config,
) *state.Machine {
	machine, err := st.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(controllerConfig, network.NewSpaceAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	hostPorts := []network.SpaceHostPorts{network.NewSpaceHostPorts(1234, "0.1.2.3")}
	err = st.SetAPIHostPorts(controllerConfig, hostPorts, hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	return machine
}
