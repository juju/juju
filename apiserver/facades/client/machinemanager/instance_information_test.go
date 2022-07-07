// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/facades/client/machinemanager/mocks"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type instanceTypesSuite struct{}

var _ = gc.Suite(&instanceTypesSuite{})

var over9kCPUCores uint64 = 9001

func (p *instanceTypesSuite) TestInstanceTypes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	leadership := mocks.NewMockLeadership(ctrl)

	backend := &mockBackend{
		cloudSpec: environscloudspec.CloudSpec{},
	}
	pool := &mockPool{}
	authorizer := testing.FakeAuthorizer{Tag: names.NewUserTag("admin"),
		Controller: true}
	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	failureCons := constraints.Value{}
	env := mockEnviron{
		results: map[constraints.Value]instances.InstanceTypesWithCostMetadata{
			itCons: {
				CostUnit:     "USD/h",
				CostCurrency: "USD",
				InstanceTypes: []instances.InstanceType{
					{Name: "instancetype-1"},
					{Name: "instancetype-2"}},
			},
		},
	}
	api, err := machinemanager.NewMachineManagerAPI(backend,
		backend,
		pool,
		machinemanager.ModelAuthorizer{
			Authorizer: authorizer,
			ModelTag:   backend.ModelTag(),
		},
		context.NewEmptyCloudCallContext(),
		common.NewResources(),
		leadership,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	cons := params.ModelInstanceTypesConstraints{
		Constraints: []params.ModelInstanceTypesConstraint{{Value: &itCons}, {Value: &failureCons}, {}},
	}
	fakeEnvironGet := func(st environs.EnvironConfigGetter,
		newEnviron environs.NewEnvironFunc,
	) (environs.Environ, error) {
		return &env, nil
	}
	r, err := machinemanager.InstanceTypes(api, fakeEnvironGet, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 3)
	expected := []params.InstanceTypesResult{
		{
			InstanceTypes: []params.InstanceType{
				{Name: "instancetype-1"},
				{Name: "instancetype-2"},
			},
			CostUnit:     "USD/h",
			CostCurrency: "USD",
		},
		{
			Error: &params.Error{
				Message: "Instances matching constraint  not found", Code: "not found",
			},
		},
		{
			Error: &params.Error{
				Message: "Instances matching constraint  not found", Code: "not found",
			},
		},
	}
	c.Assert(r.Results, gc.DeepEquals, expected)
}

type mockBackend struct {
	machinemanager.Backend
	storagecommon.StorageAccess

	cloudSpec environscloudspec.CloudSpec
}

func (st *mockBackend) VolumeAccess() storagecommon.VolumeAccess {
	return nil
}

func (st *mockBackend) FilesystemAccess() storagecommon.FilesystemAccess {
	return nil
}

func (b *mockBackend) ModelTag() names.ModelTag {
	return coretesting.ModelTag
}

func (b *mockBackend) Model() (machinemanager.Model, error) {
	return &mockModel{}, nil
}

func (b *mockBackend) CloudSpec(names.ModelTag) (environscloudspec.CloudSpec, error) {
	return b.cloudSpec, nil
}

func (b *mockBackend) Cloud(name string) (cloud.Cloud, error) {
	return cloud.Cloud{}, nil
}

func (b *mockBackend) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	return state.Credential{}, nil
}

type mockPool struct {
}

func (*mockPool) GetModel(uuid string) (machinemanager.Model, func(), error) {
	return &mockModel{}, func() {}, nil
}

func (*mockPool) SystemState() (machinemanager.ControllerBackend, error) {
	return &mockState{}, nil
}

type mockEnviron struct {
	environs.Environ
	machinemanager.Backend
	jujutesting.Stub

	results map[constraints.Value]instances.InstanceTypesWithCostMetadata
}

func (m *mockEnviron) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	it, ok := m.results[c]
	if !ok {
		return instances.InstanceTypesWithCostMetadata{}, errors.NotFoundf("Instances matching constraint %v", c)
	}
	return it, nil
}

type mockModel struct {
	machinemanager.Model
	disableOSUpgrade bool
	disableOSRefresh bool
}

func (mockModel) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	return names.NewCloudCredentialTag("foo/bob/bar"), true
}

func (mockModel) ModelTag() names.ModelTag {
	return coretesting.ModelTag
}

func (m *mockModel) Config() (*config.Config, error) {
	return config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        !m.disableOSUpgrade,
		"enable-os-refresh-update": !m.disableOSRefresh,
	}))
}

func (m *mockModel) UUID() string {
	return m.ModelTag().Id()
}

func (*mockModel) Cloud() (cloud.Cloud, error) {
	return cloud.Cloud{
		Type: "dummy",
		Regions: []cloud.Region{{
			Name: "a-region",
		}},
	}, nil
}

func (*mockModel) CloudName() string {
	return "a-cloud"
}

func (*mockModel) CloudRegion() string {
	return "a-region"
}

func (*mockModel) CloudCredential() (state.Credential, bool, error) {
	return state.Credential{}, true, nil
}
