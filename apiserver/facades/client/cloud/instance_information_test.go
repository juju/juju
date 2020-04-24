// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facades/client/cloud"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/state"
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
			itCons: {
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

	aCloud := jujucloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []jujucloud.AuthType{jujucloud.EmptyAuthType, jujucloud.UserPassAuthType},
		Regions:   []jujucloud.Region{{Name: "nether", Endpoint: "endpoint"}},
	}
	pool := &mockStatePool{
		getF: func(modelUUID string) (credentialcommon.PersistentBackend, context.ProviderCallContext, error) {
			return newModelBackend(c, aCloud, modelUUID), context.NewCloudCallContext(), nil
		},
	}
	api, err := cloud.NewCloudAPI(backend, ctlrBackend, pool, authorizer)
	c.Assert(err, jc.ErrorIsNil)

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
		{
			InstanceTypes: []params.InstanceType{
				{
					Name: "instancetype-1"},
				{Name: "instancetype-2"}},
			CostUnit:     "USD/h",
			CostCurrency: "USD",
		},
		{
			Error: &params.Error{Message: "Instances matching constraint  not found", Code: "not found"}},
		{
			Error: &params.Error{Message: "asking gce cloud information to aws cloud not valid", Code: ""}}}
	c.Assert(r.Results, gc.DeepEquals, expected)
}

func (*mockBackend) ModelConfig() (*config.Config, error) {
	return nil, nil
}

func (b *mockBackend) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	return state.Credential{}, nil
}

type mockEnviron struct {
	environs.Environ
	cloud.Backend

	results map[constraints.Value]instances.InstanceTypesWithCostMetadata
}

func (m *mockEnviron) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	it, ok := m.results[c]
	if !ok {
		return instances.InstanceTypesWithCostMetadata{}, errors.NotFoundf("Instances matching constraint %v", c)
	}
	return it, nil
}
