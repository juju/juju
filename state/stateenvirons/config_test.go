// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type environSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestGetNewEnvironFunc(c *gc.C) {
	var calls int
	var callArgs environs.OpenParams
	newEnviron := func(args environs.OpenParams) (environs.Environ, error) {
		calls++
		callArgs = args
		return nil, nil
	}
	stateenvirons.GetNewEnvironFunc(newEnviron)(s.State)
	c.Assert(calls, gc.Equals, 1)

	cfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(callArgs.Config, jc.DeepEquals, cfg)
}

func (s *environSuite) TestCloudSpec(c *gc.C) {
	owner := s.Factory.MakeUser(c, nil).UserTag()
	emptyCredential := cloud.NewEmptyCredential()
	tag := names.NewCloudCredentialTag("dummy/" + owner.Canonical() + "/empty-credential")
	err := s.State.UpdateCloudCredential(tag, emptyCredential)
	c.Assert(err, jc.ErrorIsNil)

	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:            "foo",
		CloudName:       "dummy",
		CloudCredential: tag,
		Owner:           owner,
	})
	defer st.Close()

	emptyCredential.Label = "empty-credential"
	cloudSpec, err := stateenvirons.EnvironConfigGetter{st}.CloudSpec(st.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudSpec, jc.DeepEquals, environs.CloudSpec{
		Type:             "dummy",
		Name:             "dummy",
		Region:           "dummy-region",
		Endpoint:         "dummy-endpoint",
		IdentityEndpoint: "dummy-identity-endpoint",
		StorageEndpoint:  "dummy-storage-endpoint",
		Credential:       &emptyCredential,
	})
}
