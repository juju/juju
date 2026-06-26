// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

type FirewallerAdaptersSuite struct{}

func TestFirewallerAdaptersSuite(t *testing.T) {
	tc.Run(t, &FirewallerAdaptersSuite{})
}

func (s *FirewallerAdaptersSuite) TestMachinePreservesNotFoundCode(c *tc.C) {
	adapter := &firewallerAPIAdapter{
		machineSvc: fakeMachineDomainService{getMachineUUIDErr: machineerrors.MachineNotFound},
	}

	_, err := adapter.Machine(c.Context(), names.NewMachineTag("0"))
	c.Assert(params.IsCodeNotFound(err), tc.IsTrue)
}

func (s *FirewallerAdaptersSuite) TestUnitPreservesNotFoundCode(c *tc.C) {
	adapter := &firewallerAPIAdapter{
		appSvc: fakeApplicationDomainService{getUnitLifeErr: applicationerrors.UnitNotFound},
	}

	_, err := adapter.Unit(c.Context(), names.NewUnitTag("mysql/0"))
	c.Assert(params.IsCodeNotFound(err), tc.IsTrue)
}

func (s *FirewallerAdaptersSuite) TestRelationPreservesNotFoundCode(c *tc.C) {
	adapter := &firewallerAPIAdapter{
		relationSvc: fakeRelationDomainService{getRelationUUIDErr: relationerrors.RelationNotFound},
	}

	_, err := adapter.Relation(c.Context(), names.NewRelationTag("db:db mysql:db"))
	c.Assert(params.IsCodeNotFound(err), tc.IsTrue)
}

type fakeMachineDomainService struct {
	getMachineUUIDErr error
	getMachineLifeErr error
}

func (f fakeMachineDomainService) GetMachineLife(context.Context, coremachine.Name) (life.Value, error) {
	return life.Alive, f.getMachineLifeErr
}

func (f fakeMachineDomainService) GetInstanceID(context.Context, coremachine.UUID) (instance.Id, error) {
	return "", nil
}

func (f fakeMachineDomainService) IsMachineManuallyProvisioned(context.Context, coremachine.Name) (bool, error) {
	return false, nil
}

func (f fakeMachineDomainService) GetMachineUUID(context.Context, coremachine.Name) (coremachine.UUID, error) {
	return "", f.getMachineUUIDErr
}

func (f fakeMachineDomainService) WatchModelMachines(context.Context) (watcher.StringsWatcher, error) {
	return nil, nil
}

type fakeApplicationDomainService struct {
	getUnitLifeErr error
}

func (f fakeApplicationDomainService) GetUnitLife(context.Context, coreunit.Name) (life.Value, error) {
	return life.Alive, f.getUnitLifeErr
}

type fakeRelationDomainService struct {
	getRelationUUIDErr error
	getRelationInfoErr error
}

func (f fakeRelationDomainService) GetRelationUUIDByKey(context.Context, corerelation.Key) (corerelation.UUID, error) {
	return "", f.getRelationUUIDErr
}

func (f fakeRelationDomainService) GetRelationDetails(context.Context, corerelation.UUID) (domainrelation.RelationDetails, error) {
	return domainrelation.RelationDetails{}, f.getRelationInfoErr
}

func (f fakeRelationDomainService) SetRelationErrorStatus(context.Context, corerelation.UUID, string) error {
	return nil
}

type fakeModelConfigDomainService struct{}

func (fakeModelConfigDomainService) ModelConfig(context.Context) (*config.Config, error) {
	return nil, nil
}

func (fakeModelConfigDomainService) Watch(context.Context) (watcher.StringsWatcher, error) {
	return nil, nil
}

type fakeControllerConfigDomainService struct{}

func (fakeControllerConfigDomainService) ControllerConfig(context.Context) (controller.Config, error) {
	return controller.Config{}, nil
}

type fakeNetworkDomainService struct{}

func (fakeNetworkDomainService) GetAllSpaces(context.Context) (network.SpaceInfos, error) {
	return nil, nil
}

func (fakeNetworkDomainService) WatchSubnets(context.Context, set.Strings) (watcher.StringsWatcher, error) {
	return nil, nil
}

type fakeExternalControllerDomainService struct{}

func (fakeExternalControllerDomainService) ControllerForModel(context.Context, string) (*crossmodel.ControllerInfo, error) {
	return nil, nil
}

type fakeModelInfoDomainService struct{}

func (fakeModelInfoDomainService) IsControllerModel(context.Context) (bool, error) {
	return false, nil
}

var _ MachineDomainService = fakeMachineDomainService{}
var _ ApplicationDomainService = fakeApplicationDomainService{}
var _ RelationDomainService = fakeRelationDomainService{}
var _ ModelConfigDomainService = fakeModelConfigDomainService{}
var _ ControllerConfigDomainService = fakeControllerConfigDomainService{}
var _ NetworkDomainService = fakeNetworkDomainService{}
var _ ExternalControllerDomainService = fakeExternalControllerDomainService{}
var _ ModelInfoDomainService = fakeModelInfoDomainService{}
