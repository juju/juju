// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"testing"

	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// baseSuite is the foundation for test suites in this package.
type BaseSuite struct {
	jujutesting.IsolationSuite

	Manager *residentManager
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Manager = newResidentManager()
}

func (s *BaseSuite) NewResident() *resident {
	return s.Manager.new()
}

// entitySuite is the base suite for testing cached entities
// (models, applications, machines).
type EntitySuite struct {
	BaseSuite

	Gauges *ControllerGauges
	Hub    *pubsub.SimpleHub
}

func (s *EntitySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.TRACE)
	s.Hub = pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
		Logger: logger,
	})

	s.Gauges = createControllerGauges()
}

func (s *EntitySuite) NewModel(details ModelChange) *Model {
	m := newModel(s.Gauges, s.Hub, s.Manager.new())
	m.setDetails(details)
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
