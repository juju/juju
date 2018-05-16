// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/collections/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/core/auditlog"
)

type auditConfigSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&auditConfigSuite{})

func (s *auditConfigSuite) TestUsesGetAuditConfig(c *gc.C) {
	var calls int
	s.config.GetAuditConfig = func() auditlog.Config {
		calls++
		return auditlog.Config{
			Enabled:        true,
			ExcludeMethods: set.NewStrings("Midlake.Bandits"),
		}
	}

	srv := s.newServer(c, s.config)

	auditConfig := srv.GetAuditConfig()
	c.Assert(auditConfig, gc.DeepEquals, auditlog.Config{
		Enabled:        true,
		ExcludeMethods: set.NewStrings("Midlake.Bandits"),
	})
	c.Assert(calls, gc.Equals, 1)
}

func (s *auditConfigSuite) TestNewServerValidatesConfig(c *gc.C) {
	s.config.GetAuditConfig = nil

	srv, err := apiserver.NewServer(s.config)
	c.Assert(err, gc.ErrorMatches, "missing GetAuditConfig not valid")
	c.Assert(srv, gc.IsNil)
}
