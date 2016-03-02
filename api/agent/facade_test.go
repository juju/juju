// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestLifeCallError(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeNoResult(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeOversizedResult(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeRandomError(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeErrNotFound(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeErrUnauthorized(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeUnknown(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeAlive(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeDying(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestLifeDead(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordCallError(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordNoResult(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordOversizedResult(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordRandomError(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordErrDead(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordErrNotFound(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordErrUnauthorized(c *gc.C) {
	c.Fatalf("xxx")
}

func (s *FacadeSuite) TestSetPasswordSuccess(c *gc.C) {
	c.Fatalf("xxx")
}
