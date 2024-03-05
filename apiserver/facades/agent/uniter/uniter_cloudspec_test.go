// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
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

func (s *cloudSpecUniterSuite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.SetUpTest(c)

	// Update the application config for wordpress so that it is authorised to
	// retrieve its cloud spec.
	conf := map[string]any{coreapplication.TrustConfigOptionName: true}
	fields := map[string]environschema.Attr{coreapplication.TrustConfigOptionName: {Type: environschema.Tbool}}
	defaults := map[string]any{coreapplication.TrustConfigOptionName: false}
	err := s.wordpress.UpdateApplicationConfig(conf, nil, fields, defaults)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudSpecUniterSuite) TestGetCloudSpecReturnsSpecWhenTrusted(c *gc.C) {
	serviceFactory := s.ControllerServiceFactory(c)

	facadeContext := s.facadeContext(c)
	uniterAPI, err := uniter.NewUniterAPIWithServices(context.Background(), facadeContext, serviceFactory.ControllerConfig(), serviceFactory.Cloud(), serviceFactory.Credential(), serviceFactory.Unit())
	c.Assert(err, jc.ErrorIsNil)
	result, err := uniterAPI.CloudSpec(context.Background())
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
	_, cm, _, _ := s.setupCAASModel(c)

	facadeContext := s.facadeContext(c)
	facadeContext.State_ = cm.State()

	serviceFactory := facadeContext.ServiceFactory()

	uniterAPI, err := uniter.NewUniterAPIWithServices(context.Background(), facadeContext, serviceFactory.ControllerConfig(), serviceFactory.Cloud(), serviceFactory.Credential(), serviceFactory.Unit())
	c.Assert(err, jc.ErrorIsNil)
	uniter.SetNewContainerBrokerFunc(uniterAPI, func(context.Context, environs.OpenParams) (caas.Broker, error) {
		return &fakeBroker{}, nil
	})

	result, err := uniterAPI.CloudAPIVersion(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{
		Result: "6.66",
	})
}
