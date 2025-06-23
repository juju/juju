// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

type modelStatusSuite struct {
	testing.BaseSuite

	Owner names.UserTag

	controllerUUID uuid.UUID

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machineService *MockMachineService
	statusService  *MockStatusService
}

func TestModelStatusSuite(t *stdtesting.T) {
	tc.Run(t, &modelStatusSuite{})
}

func (s *modelStatusSuite) SetUpTest(c *tc.C) {
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	controllerUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	s.controllerUUID = controllerUUID
}

func (s *modelStatusSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Full multi-model status success case for IAAS.
- Full multi-model status success case for CAAS.
- ModelStatus without authorisation denied.
- ModelStatus as owner allowed.
- ModelStatus validates all model tag params.
`)
}

type noopStoragePoolGetter struct{}

func (noopStoragePoolGetter) GetStorageRegistry(_ context.Context) (storage.ProviderRegistry, error) {
	return storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}, nil
}

func (noopStoragePoolGetter) GetStoragePoolByName(_ context.Context, name string) (domainstorage.StoragePool, error) {
	return domainstorage.StoragePool{}, fmt.Errorf("storage pool %q not found%w", name, errors.Hide(storageerrors.PoolNotFoundError))
}

type statePolicy struct{}

func (statePolicy) ConstraintsValidator(context.Context) (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (statePolicy) StorageServices() (state.StoragePoolGetter, error) {
	return noopStoragePoolGetter{}, nil
}
