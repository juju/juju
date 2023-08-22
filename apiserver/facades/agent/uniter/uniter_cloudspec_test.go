// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdcontext "context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

type cloudSpecUniterSuite struct {
	uniterSuiteBase
}

var _ = gc.Suite(&cloudSpecUniterSuite{})

func (s *cloudSpecUniterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.uniterSuiteBase.setupMocks(c)

	// Update the application config for wordpress so that it is authorised to
	// retrieve its cloud spec.
	conf := map[string]interface{}{coreapplication.TrustConfigOptionName: true}
	fields := map[string]environschema.Attr{coreapplication.TrustConfigOptionName: {Type: environschema.Tbool}}
	defaults := map[string]interface{}{coreapplication.TrustConfigOptionName: false}
	err := s.wordpress.UpdateApplicationConfig(conf, nil, fields, defaults)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *cloudSpecUniterSuite) TestGetCloudSpecReturnsSpecWhenTrusted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	result, err := s.uniter.CloudSpec(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result.Name, gc.Equals, "dummy")

	exp := map[string]string{
		"username": "dummy",
		"password": "secret",
	}
	c.Assert(result.Result.Credential.Attributes, gc.DeepEquals, exp)
}

func (s *cloudSpecUniterSuite) TestCloudAPIVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, cm, _, _ := s.setupCAASModel(c)

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)
	uniter.SetNewContainerBrokerFunc(uniterAPI, func(stdcontext.Context, environs.OpenParams) (caas.Broker, error) {
		return &fakeBroker{}, nil
	})

	result, err := uniterAPI.CloudAPIVersion(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{
		Result: "6.66",
	})
}
