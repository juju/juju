// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce/google"
)

type connConfigSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&connConfigSuite{})

func (*connConfigSuite) TestValidateValid(c *gc.C) {
	cfg := google.ConnectionConfig{
		Region:    "spam",
		ProjectID: "eggs",
	}
	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*connConfigSuite) TestValidateMissingRegion(c *gc.C) {
	cfg := google.ConnectionConfig{
		ProjectID: "eggs",
	}
	err := cfg.Validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValueError{})
	c.Check(err.(*config.InvalidConfigValueError).Key, gc.Equals, "GCE_REGION")
}

func (*connConfigSuite) TestValidateMissingProjectID(c *gc.C) {
	cfg := google.ConnectionConfig{
		Region: "spam",
	}
	err := cfg.Validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValueError{})
	c.Check(err.(*config.InvalidConfigValueError).Key, gc.Equals, "GCE_PROJECT_ID")
}

func (s *connSuite) TestConnect(c *gc.C) {
	google.SetRawConn(s.Conn, nil)
	service := &compute.Service{}
	s.PatchValue(google.NewRawConnection, func(auth google.Credentials) (*compute.Service, error) {
		return service, nil
	})

	conn, err := s.ConnCfg.Connect(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(google.ExposeRawService(conn), gc.Equals, service)
}
