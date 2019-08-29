// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
)

type PodSpecSuite struct {
	CAASFixture

	Model   *state.CAASModel
	State   *state.State
	Factory *factory.Factory

	application *state.Application
}

var _ = gc.Suite(&PodSpecSuite{})

func (s *PodSpecSuite) SetUpTest(c *gc.C) {
	s.CAASFixture.SetUpTest(c)
	s.Model, s.State = s.newCAASModel(c)
	s.Factory = factory.NewFactory(s.State, s.StatePool)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	ch := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

}

func (s *PodSpecSuite) assertPodSpec(c *gc.C, tag names.ApplicationTag, expect string) {
	spec, err := s.Model.PodSpec(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, gc.Equals, spec)
}

func (s *PodSpecSuite) assertPodSpecNotFound(c *gc.C, tag names.ApplicationTag) {
	_, err := s.Model.PodSpec(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *PodSpecSuite) TestSetPodSpecApplication(c *gc.C) {
	err := s.Model.SetPodSpec(s.application.ApplicationTag(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	s.assertPodSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetPodSpecApplicationDying(c *gc.C) {
	// create a unit to prevent app from being removed
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = s.Model.SetPodSpec(s.application.ApplicationTag(), "foo")
	c.Assert(err, gc.ErrorMatches, "application gitlab not alive")
	s.assertPodSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetPodSpecUpdates(c *gc.C) {
	for _, spec := range []string{"spec0", "spec1"} {
		err := s.Model.SetPodSpec(s.application.ApplicationTag(), spec)
		c.Assert(err, jc.ErrorIsNil)
		s.assertPodSpec(c, s.application.ApplicationTag(), spec)
	}
}

func (s *PodSpecSuite) TestRemoveApplicationRemovesPodSpec(c *gc.C) {
	err := s.Model.SetPodSpec(s.application.ApplicationTag(), "spec")
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPodSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestWatchPodSpec(c *gc.C) {
	w, err := s.Model.WatchPodSpec(s.application.ApplicationTag())
	c.Assert(err, jc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// No spec -> spec set.
	err = s.Model.SetPodSpec(s.application.ApplicationTag(), "spec0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// No change.
	err = s.Model.SetPodSpec(s.application.ApplicationTag(), "spec0")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Multiple changes coalesced.
	err = s.Model.SetPodSpec(s.application.ApplicationTag(), "spec1")
	c.Assert(err, jc.ErrorIsNil)
	err = s.Model.SetPodSpec(s.application.ApplicationTag(), "spec2")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
