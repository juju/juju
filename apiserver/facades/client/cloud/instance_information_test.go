// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
)

type instanceTypesSuite struct{}

var _ = gc.Suite(&instanceTypesSuite{})

var over9kCPUCores uint64 = 9001

func (p *instanceTypesSuite) TestInstanceTypes(c *gc.C) {
	backend := &mockBackend{}
	ctlrBackend := &mockBackend{
		cloud: jujucloud.Cloud{Name: "aws"},
	}
	authorizer := &testing.FakeAuthorizer{Tag: names.NewUserTag("admin"),
		Controller: true}

	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	env := &mockEnviron{
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
	fakeEnvironGet := func(
		st environs.EnvironConfigGetter,
		newEnviron environs.NewEnvironFunc,
	) (environs.Environ, error) {
		return env, nil
	}

	api := cloud.NewCloudTestingAPI(backend, ctlrBackend, authorizer)

	failureCons := constraints.Value{}
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

func (*mockBackend) ModelConfig() (*config.Config, error) {
	return nil, nil
}

func (b *mockBackend) CloudCredential(tag names.CloudCredentialTag) (jujucloud.Credential, error) {
	return jujucloud.Credential{}, nil
}

type mockEnviron struct {
	environs.Environ
	cloud.Backend

	results map[constraints.Value]instances.InstanceTypesWithCostMetadata
}

func (m *mockEnviron) InstanceTypes(c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	it, ok := m.results[c]
	if !ok {
		return instances.InstanceTypesWithCostMetadata{}, errors.NotFoundf("Instances matching constraint %v", c)
	}
	return it, nil
}
