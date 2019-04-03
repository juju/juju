// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"testing"

	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// baseSuite is the foundation for test suites in this package.
type baseSuite struct {
	jujutesting.IsolationSuite

	gauges *cache.ControllerGauges
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.gauges = cache.CreateControllerGauges()
}

// entitySuite is the base suite for testing cached entities
// (models, applications, machines).
type entitySuite struct {
	baseSuite

	hub *pubsub.SimpleHub
}

func (s *entitySuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	s.hub = pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
		Logger: logger,
	})
}

func (s *entitySuite) newModel(details cache.ModelChange) *cache.Model {
	m := cache.NewModel(s.gauges, s.hub)
	m.SetDetails(details)
	return m
}

type ImportSuite struct{}

var _ = gc.Suite(&ImportSuite{})

func (*ImportSuite) TestImports(c *gc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/cache")

	// This package only brings in other core packages.
	c.Assert(found, jc.SameContents, []string{
		"core/constraints",
		"core/instance",
		"core/life",
		"core/lxdprofile",
		"core/network",
		"core/status",
	})
}
