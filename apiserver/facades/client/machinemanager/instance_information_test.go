// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
)

type instanceTypesSuite struct{}

var _ = gc.Suite(&instanceTypesSuite{})

var over9kCPUCores uint64 = 9001

func (p *instanceTypesSuite) TestInstanceTypes(c *gc.C) {
	backend := &mockBackend{
		cloudSpec: environs.CloudSpec{},
	}
	pool := &mockPool{}
	authorizer := testing.FakeAuthorizer{Tag: names.NewUserTag("admin"),
		Controller: true}
	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	failureCons := constraints.Value{}
	env := mockEnviron{
		results: map[constraints.Value]instances.InstanceTypesWithCostMetadata{
			itCons: instances.InstanceTypesWithCostMetadata{
				CostUnit:     "USD/h",
				CostCurrency: "USD",
				InstanceTypes: []instances.InstanceType{
					{Name: "instancetype-1"},
					{Name: "instancetype-2"}},
			},
		},
	}
	api, err := machinemanager.NewMachineManagerAPI(backend, pool, authorizer)
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
		params.InstanceTypesResult{
			InstanceTypes: []params.InstanceType{
				params.InstanceType{
					Name: "instancetype-1",
				},
				params.InstanceType{
					Name: "instancetype-2",
				}},
			CostUnit:     "USD/h",
			CostCurrency: "USD",
		},
		params.InstanceTypesResult{
			Error: &params.Error{Message: "Instances matching constraint  not found", Code: "not found"}},
		params.InstanceTypesResult{
			Error: &params.Error{Message: "Instances matching constraint  not found", Code: "not found"}}}
	c.Assert(r.Results, gc.DeepEquals, expected)
}

type mockBackend struct {
	machinemanager.Backend

	cloudSpec environs.CloudSpec
}

func (b *mockBackend) ModelTag() names.ModelTag {
	return names.NewModelTag("beef1beef1-0000-0000-000011112222")
}

func (b *mockBackend) Model() (machinemanager.Model, error) {
	return &mockModel{}, nil
}

func (b *mockBackend) CloudSpec(names.ModelTag) (environs.CloudSpec, error) {
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

type mockEnviron struct {
	environs.Environ
	machinemanager.Backend
	jujutesting.Stub

	results map[constraints.Value]instances.InstanceTypesWithCostMetadata
}

func (m *mockEnviron) InstanceTypes(c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	it, ok := m.results[c]
	if !ok {
		return instances.InstanceTypesWithCostMetadata{}, errors.NotFoundf("Instances matching constraint %v", c)
	}
	return it, nil
}

type mockModel struct {
	machinemanager.Model
}

func (mockModel) CloudCredential() (names.CloudCredentialTag, bool) {
	return names.NewCloudCredentialTag("foo/bob/bar"), true
}

func (mockModel) ModelTag() names.ModelTag {
	return names.NewModelTag("beef1beef1-0000-0000-000011112222")
}

func (*mockModel) Config() (*config.Config, error) {
	return config.New(config.UseDefaults, dummy.SampleConfig())
}

func (*mockModel) Cloud() string {
	return "a-cloud"
}

func (*mockModel) CloudRegion() string {
	return "a-region"
}
