// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/facades/client/application"
)

type cloudNativeUniterSuite struct {
	uniterSuite
}

var _ = gc.Suite(&cloudNativeUniterSuite{})

func (s *cloudNativeUniterSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	// Ensure the application is not trusted prior to tests.
	s.setApplicationTrust(c, false)
}

// setApplicationTrust updates the configuration for the application unit to
// allow or deny access for cloud spec retrieval.
func (s *cloudNativeUniterSuite) setApplicationTrust(c *gc.C, trusted bool) {
	conf := map[string]interface{}{application.TrustConfigOptionName: trusted}
	fields := map[string]environschema.Attr{application.TrustConfigOptionName: {Type: environschema.Tbool}}
	defaults := map[string]interface{}{application.TrustConfigOptionName: false}
	err := s.wordpressApplication.UpdateApplicationConfig(conf, nil, fields, defaults)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudNativeUniterSuite) TestCloudSpecErrorWhenUnauthorized(c *gc.C) {
	result, err := s.uniter.CloudSpec()
	c.Check(err, gc.ErrorMatches, "permission denied")
	c.Check(result, gc.IsNil)
}

func (s *cloudNativeUniterSuite) TestGetCloudSpecReturnsSpecWhenTrusted(c *gc.C) {
	s.setApplicationTrust(c, true)

	result, err := s.uniter.CloudSpec()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Name, gc.Equals, "dummy")

	exp := map[string]string{
		"username": "dummy",
		"password": "secret",
	}
	c.Check(result.Credential.Attributes, gc.DeepEquals, exp)
}
