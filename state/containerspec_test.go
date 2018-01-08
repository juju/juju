// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
)

type ContainerSpecSuite struct {
	CAASFixture

	Model   *state.CAASModel
	State   *state.State
	Factory *factory.Factory
}

var _ = gc.Suite(&ContainerSpecSuite{})

func (s *ContainerSpecSuite) SetUpTest(c *gc.C) {
	s.CAASFixture.SetUpTest(c)
	s.Model, s.State = s.newCAASModel(c)
	s.Factory = factory.NewFactory(s.State)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
}

func (s *ContainerSpecSuite) assertContainerSpec(c *gc.C, tag names.Tag, expect string) {
	spec, err := s.Model.ContainerSpec(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, gc.Equals, spec)
}

func (s *ContainerSpecSuite) assertContainerSpecNotFound(c *gc.C, tag names.Tag) {
	_, err := s.Model.ContainerSpec(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ContainerSpecSuite) TestSetContainerSpecApplication(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	err := s.Model.SetContainerSpec(app.Tag(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	s.assertContainerSpec(c, app.Tag(), "foo")
}

func (s *ContainerSpecSuite) TestSetContainerSpecApplicationDying(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	// create a unit to prevent app from being removed
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})
	err := app.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.SetContainerSpec(app.Tag(), "foo")
	c.Assert(err, gc.ErrorMatches, "application mysql not alive")
	s.assertContainerSpecNotFound(c, app.Tag())
}

func (s *ContainerSpecSuite) TestSetContainerSpecUnit(c *gc.C) {
	u := s.Factory.MakeUnit(c, nil)
	err := s.Model.SetContainerSpec(u.Tag(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	s.assertContainerSpec(c, u.Tag(), "foo")
}

func (s *ContainerSpecSuite) TestSetContainerSpecUnitNotFound(c *gc.C) {
	u := s.Factory.MakeUnit(c, nil)
	// TODO(caas) destroying the unit will remove it, because
	// it is not assigned to a machine. We'll need to prevent
	// CAAS units from being removed immediately, as they will
	// never be assigned to machines.
	err := u.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.SetContainerSpec(u.Tag(), "foo")
	c.Assert(err, gc.ErrorMatches, `unit "mysql/0" not found`)
	s.assertContainerSpecNotFound(c, u.Tag())
}

func (s *ContainerSpecSuite) TestSetContainerSpecInvalidEntity(c *gc.C) {
	err := s.Model.SetContainerSpec(names.NewMachineTag("0"), "foo")
	c.Assert(err, gc.ErrorMatches, "setting container spec for machine entity not supported")
}

func (s *ContainerSpecSuite) TestContainerSpecHierarchy(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	u0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})
	u1 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})

	err := s.Model.SetContainerSpec(app.Tag(), "app-spec")
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.SetContainerSpec(u0.Tag(), "u0-spec")
	c.Assert(err, jc.ErrorIsNil)

	s.assertContainerSpec(c, app.Tag(), "app-spec")
	s.assertContainerSpec(c, u0.Tag(), "u0-spec")
	s.assertContainerSpec(c, u1.Tag(), "app-sec")
}

func (s *ContainerSpecSuite) TestSetContainerSpecUpdates(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	for _, spec := range []string{"spec0", "spec1"} {
		err := s.Model.SetContainerSpec(app.Tag(), spec)
		c.Assert(err, jc.ErrorIsNil)
		s.assertContainerSpec(c, app.Tag(), spec)
	}
}

func (s *ContainerSpecSuite) TestRemoveApplicationRemovesContainerSpec(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	err := s.Model.SetContainerSpec(app.Tag(), "spec")
	c.Assert(err, jc.ErrorIsNil)

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertContainerSpecNotFound(c, app.Tag())
}

func (s *ContainerSpecSuite) TestRemoveUnitRemovesContainerSpec(c *gc.C) {
	u := s.Factory.MakeUnit(c, nil)
	err := s.Model.SetContainerSpec(u.Tag(), "spec")
	c.Assert(err, jc.ErrorIsNil)

	err = u.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertContainerSpecNotFound(c, u.Tag())
}

func (s *ContainerSpecSuite) TestWatchContainerSpecApplication(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	w, err := s.Model.WatchContainerSpec(app.Tag())
	c.Assert(err, jc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// No spec -> spec set.
	err = s.Model.SetContainerSpec(app.Tag(), "spec0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// No change.
	err = s.Model.SetContainerSpec(app.Tag(), "spec0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Multiple changes coalesced.
	err = s.Model.SetContainerSpec(app.Tag(), "spec1")
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.SetContainerSpec(app.Tag(), "spec2")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *ContainerSpecSuite) TestWatchContainerSpecUnit(c *gc.C) {
	app := s.Factory.MakeApplication(c, nil)
	u := s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})
	w, err := s.Model.WatchContainerSpec(u.Tag())
	c.Assert(err, jc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Set application container spec.
	err = s.Model.SetContainerSpec(app.Tag(), "spec0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set unit container spec. It's the same as the
	// application's, but still triggers a change as
	// it's from a different doc.
	//
	// TODO(caas) we should not trigger a change in this
	// case, as it will cause unnecessary worker activity.
	err = s.Model.SetContainerSpec(u.Tag(), "spec0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Changing the application's container spec triggers
	// a change even if there's a unit container spec.
	//
	// TODO(caas) we should not trigger a change in this
	// case, as it will cause unnecessary worker activity.
	err = s.Model.SetContainerSpec(app.Tag(), "spec1")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
