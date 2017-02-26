// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/dummy"
)

type instanceTypesSuite struct{}

var _ = gc.Suite(&instanceTypesSuite{})

var over9kCPUCores uint64 = 9001

func (p *instanceTypesSuite) TestInstanceTypes(c *gc.C) {
	backend := mockBackend{
		cloudSpec: environs.CloudSpec{},
	}
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
	api := cloud.NewCloudTestingAPI(&backend, authorizer)

	cons := params.CloudInstanceTypesConstraints{
		Constraints: []params.CloudInstanceTypesConstraint{
			{CloudTag: "cloud-aws",
				CloudRegion: "a-region",
				Constraints: &itCons},
			{CloudTag: "cloud-aws",
				CloudRegion: "a-region",
				Constraints: &failureCons},
			{CloudTag: "cloud-gce",
				CloudRegion: "a-region",
				Constraints: &itCons}},
	}
	fakeEnvironGet := func(st environs.EnvironConfigGetter,
		newEnviron environs.NewEnvironFunc,
	) (environs.Environ, error) {
		return &env, nil
	}
	r, err := cloud.InstanceTypes(api, fakeEnvironGet, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 3)
	expected := []params.InstanceTypesResult{
		params.InstanceTypesResult{
			InstanceTypes: []params.InstanceType{
				params.InstanceType{
					Name: "instancetype-1"},
				params.InstanceType{Name: "instancetype-2"}},
			CostUnit:     "USD/h",
			CostCurrency: "USD",
		},
		params.InstanceTypesResult{
			Error: &params.Error{Message: "Instances matching constraint  not found", Code: "not found"}},
		params.InstanceTypesResult{
			Error: &params.Error{Message: "asking gce cloud information to aws cloud not valid", Code: ""}}}
	c.Assert(r.Results, gc.DeepEquals, expected)
}

func (*mockBackend) GetModel(t names.ModelTag) (cloud.Model, error) {
	return &mockModel{cloud: "aws"}, nil
}

func (*mockBackend) ModelConfig() (*config.Config, error) {
	return nil, nil
}

func (fb *mockBackend) CloudSpec(names.ModelTag) (environs.CloudSpec, error) {
	fb.MethodCall(fb, "CloudSpec")
	if err := fb.NextErr(); err != nil {
		return environs.CloudSpec{}, err
	}
	return fb.cloudSpec, nil
}

func (fb *mockBackend) CloudCredential(tag names.CloudCredentialTag) (jujucloud.Credential, error) {
	return jujucloud.Credential{}, nil
}

type mockEnviron struct {
	environs.Environ
	cloud.Backend
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

func (mockModel) ModelTag() names.ModelTag {
	return names.NewModelTag("beef1beef1-0000-0000-000011112222")
}

func (*mockModel) Config() (*config.Config, error) {
	return config.New(config.UseDefaults, dummy.SampleConfig())
}
