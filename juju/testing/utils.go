// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	coreuser "github.com/juju/juju/core/user"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/objectstore"
	objectstoretesting "github.com/juju/juju/internal/objectstore/testing"
	"github.com/juju/juju/state"
)

const AdminSecret = "dummy-secret"

var (
	// AdminUser is the default admin user for a controller.
	AdminUser = names.NewUserTag("admin")
	AdminName = coreuser.NameFromTag(AdminUser)

	// DefaultCloudRegion is the default cloud region for a controller model.
	DefaultCloudRegion = "dummy-region"

	// DefaultCloud is the default cloud for a controller model.
	DefaultCloud = cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Regions:   []cloud.Region{{Name: DefaultCloudRegion}},
	}

	// DefaultCredentialTag is the default credential for all models.
	DefaultCredentialTag = names.NewCloudCredentialTag("dummy/admin/default")

	// DefaultCredentialId is the default credential id for all models.
	DefaultCredentialId = corecredential.KeyFromTag(DefaultCredentialTag)
)

// NewObjectStore creates a new object store for testing.
// This uses the memory metadata service.
func NewObjectStore(c *gc.C, modelUUID string) coreobjectstore.ObjectStore {
	return NewObjectStoreWithMetadataService(c, modelUUID, objectstoretesting.MemoryMetadataService())
}

// NewObjectStoreWithMetadataService creates a new object store for testing.
func NewObjectStoreWithMetadataService(c *gc.C, modelUUID string, metadataService objectstore.MetadataService) coreobjectstore.ObjectStore {
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
	c *gc.C,
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

func MacaroonEquals(c *gc.C, m1, m2 *macaroon.Macaroon) {
	c.Assert(m1.Id(), jc.DeepEquals, m2.Id())
	c.Assert(m1.Signature(), jc.DeepEquals, m2.Signature())
	c.Assert(m1.Location(), gc.Equals, m2.Location())
}

func MacaroonsEqual(c *gc.C, ms1, ms2 []macaroon.Slice) error {
	if len(ms1) != len(ms2) {
		return errors.Errorf("length mismatch, %d vs %d", len(ms1), len(ms2))
	}

	for i := 0; i < len(ms1); i++ {
		m1 := ms1[i]
		m2 := ms2[i]
		if len(m1) != len(m2) {
			return errors.Errorf("length mismatch, %d vs %d", len(m1), len(m2))
		}
		for i := 0; i < len(m1); i++ {
			MacaroonEquals(c, m1[i], m2[i])
		}
	}
	return nil
}

func NewMacaroon(id string) (*macaroon.Macaroon, error) {
	return macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
}

func MustNewMacaroon(id string) *macaroon.Macaroon {
	mac, err := NewMacaroon(id)
	if err != nil {
		panic(err)
	}
	return mac
}

// InsertDummyCloudType is a db bootstrap option which inserts the dummy cloud type.
func InsertDummyCloudType(ctx context.Context, controller, model database.TxnRunner) error {
	return cloudstate.AllowCloudType(ctx, controller, 666, "dummy")
}
