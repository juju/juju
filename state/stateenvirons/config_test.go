// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type environSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&environSuite{})

var testCloud = cloud.Cloud{
	Name:              "dummy",
	Type:              "dummy",
	AuthTypes:         []cloud.AuthType{cloud.EmptyAuthType, cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	Regions:           []cloud.Region{{Name: "dummy-region"}},
	Endpoint:          "dummy-endpoint",
	IdentityEndpoint:  "dummy-identity-endpoint",
	StorageEndpoint:   "dummy-storage-endpoint",
	IsControllerCloud: true,
}

func (s *environSuite) TestGetNewEnvironFunc(c *gc.C) {
	var calls int
	var callArgs environs.OpenParams
	newEnviron := func(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
		calls++
		callArgs = args
		return nil, nil
	}
	_, err := stateenvirons.GetNewEnvironFunc(newEnviron)(s.Model, &cloudGetter{cloud: &testCloud}, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(calls, gc.Equals, 1)

	cfg, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callArgs.Config, jc.DeepEquals, cfg)
}

type credentialGetter struct {
	cred *cloud.Credential
}

func (c credentialGetter) CloudCredential(_ context.Context, id credential.ID) (cloud.Credential, error) {
	if c.cred == nil {
		return cloud.Credential{}, errors.NotFoundf("credential %q", id)
	}
	return *c.cred, nil
}

type cloudGetter struct {
	cloud *cloud.Cloud
}

func (c cloudGetter) Get(_ context.Context, name string) (*cloud.Cloud, error) {
	if c.cloud == nil {
		return nil, errors.NotFoundf("cloud %q", name)
	}
	return c.cloud, nil
}

func (s *environSuite) TestCloudSpec(c *gc.C) {
	owner := s.Factory.MakeUser(c, nil).UserTag()
	emptyCredential := cloud.NewEmptyCredential()
	tag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/empty-credential")

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "foo",
		CloudName:       "dummy",
		CloudCredential: tag,
		Owner:           owner,
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	emptyCredential.Label = "empty-credential"
	cloudSpec, err := stateenvirons.EnvironConfigGetter{
		Model:             m,
		CloudService:      &cloudGetter{cloud: &testCloud},
		CredentialService: &credentialGetter{cred: &emptyCredential}}.CloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudSpec, jc.DeepEquals, environscloudspec.CloudSpec{
		Type:              "dummy",
		Name:              "dummy",
		Region:            "dummy-region",
		Endpoint:          "dummy-endpoint",
		IdentityEndpoint:  "dummy-identity-endpoint",
		StorageEndpoint:   "dummy-storage-endpoint",
		Credential:        &emptyCredential,
		IsControllerCloud: true,
	})
}

func (s *environSuite) TestCloudSpecForModel(c *gc.C) {
	owner := s.Factory.MakeUser(c, nil).UserTag()
	emptyCredential := cloud.NewEmptyCredential()
	tag := names.NewCloudCredentialTag("dummy/" + owner.Id() + "/empty-credential")

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "foo",
		CloudName:       "dummy",
		CloudCredential: tag,
		Owner:           owner,
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	emptyCredential.Label = "empty-credential"
	cloudSpec, err := stateenvirons.CloudSpecForModel(
		context.Background(), m,
		&cloudGetter{cloud: &testCloud},
		&credentialGetter{cred: &emptyCredential})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudSpec, jc.DeepEquals, environscloudspec.CloudSpec{
		Type:              "dummy",
		Name:              "dummy",
		Region:            "dummy-region",
		Endpoint:          "dummy-endpoint",
		IdentityEndpoint:  "dummy-identity-endpoint",
		StorageEndpoint:   "dummy-storage-endpoint",
		Credential:        &emptyCredential,
		IsControllerCloud: true,
	})
}

func (s *environSuite) TestGetNewCAASBrokerFunc(c *gc.C) {
	var calls int
	var callArgs environs.OpenParams
	newBroker := func(_ context.Context, args environs.OpenParams) (caas.Broker, error) {
		calls++
		callArgs = args
		return nil, nil
	}
	_, err := stateenvirons.GetNewCAASBrokerFunc(newBroker)(
		s.Model, &cloudGetter{cloud: &testCloud}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(calls, gc.Equals, 1)

	cfg, err := s.Model.ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callArgs.Config, jc.DeepEquals, cfg)
}

type fakeBroker struct {
	caas.Broker
}

func (*fakeBroker) APIVersion() (string, error) {
	return "6.66", nil
}

func (s *environSuite) TestCloudAPIVersion(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		Name: "foo",
	})
	defer st.Close()

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	cred := cloud.NewNamedCredential("dummy-credential", "userpass", nil, false)
	newBrokerFunc := func(_ context.Context, args environs.OpenParams) (caas.Broker, error) {
		c.Assert(args.Cloud, jc.DeepEquals, environscloudspec.CloudSpec{
			Name:       "caascloud",
			Type:       "kubernetes",
			Credential: &cred,
		})
		return &fakeBroker{}, nil
	}

	envConfigGetter := stateenvirons.EnvironConfigGetter{
		Model: m, NewContainerBroker: newBrokerFunc,
		CloudService:      &cloudGetter{cloud: &cloud.Cloud{Name: "caascloud", Type: "kubernetes"}},
		CredentialService: &credentialGetter{cred: &cred}}
	cloudSpec, err := envConfigGetter.CloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	apiVersion, err := envConfigGetter.CloudAPIVersion(cloudSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiVersion, gc.Equals, "6.66")
}
