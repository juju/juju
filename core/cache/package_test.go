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

func (s *BaseSuite) NewController(config ControllerConfig) (*Controller, error) {
	return newController(config, s.Manager)
}

func (s *BaseSuite) NewResident() *Resident {
	return s.Manager.new()
}

func (s *BaseSuite) AssertResident(c *gc.C, id uint64, expectPresent bool) {
	_, present := s.Manager.residents[id]
	c.Assert(present, gc.Equals, expectPresent)
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

func (s *EntitySuite) NewApplication(details ApplicationChange) *Application {
	a := newApplication(s.Gauges, s.Hub, s.NewResident())
	a.SetDetails(details)
	return a
}

func (s *BaseSuite) AssertWorkerResource(c *gc.C, resident *Resident, id uint64, expectPresent bool) {
	_, present := resident.workers[id]
	c.Assert(present, gc.Equals, expectPresent)
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
