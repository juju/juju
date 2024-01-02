// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
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
	c.Assert(spec, gc.Equals, expect)
}

func (s *PodSpecSuite) assertRawK8sSpec(c *gc.C, tag names.ApplicationTag, expect string) {
	rs, err := s.Model.RawK8sSpec(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rs, gc.Equals, expect)
}

func (s *PodSpecSuite) assertPodSpecNotFound(c *gc.C, tag names.ApplicationTag) {
	_, err := s.Model.PodSpec(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *PodSpecSuite) applySetRawK8sSpecOperation(token leadership.Token, appTag names.ApplicationTag, spec *string) error {
	modelOp := s.Model.SetRawK8sSpecOperation(token, appTag, spec)
	return s.State.ApplyOperation(modelOp)
}

func (s *PodSpecSuite) assertRawK8sSpecNotFound(c *gc.C, tag names.ApplicationTag) {
	_, err := s.Model.RawK8sSpec(tag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplication(c *gc.C) {
	err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertRawK8sSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplicationOperator(c *gc.C) {
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, gc.ErrorMatches, "cannot set k8s spec on an operator charm")
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplicationDying(c *gc.C) {
	// create a unit to prevent app from being removed
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.application, state.Dying)

	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertRawK8sSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationApplicationDead(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	c.Assert(unit.Remove(), jc.ErrorIsNil)
	assertCleanupCount(c, s.State, 1)
	assertLife(c, s.application, state.Dead)

	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, gc.ErrorMatches, "setting pod-spec on dead application gitlab not valid")
	s.assertRawK8sSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetRawK8sSpecOperationUpdates(c *gc.C) {
	for _, spec := range []string{"spec0", "spec1"} {
		err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), &spec)
		c.Assert(err, jc.ErrorIsNil)
		s.assertRawK8sSpec(c, s.application.ApplicationTag(), spec)
	}
}

func (s *PodSpecSuite) TestRemoveApplicationRemovesRawK8sSpec(c *gc.C) {
	err := s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("spec"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// App removal requires cluster resources to be cleared.
	err = s.application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, s.State, 2)

	s.assertRawK8sSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetPodSpecApplication(c *gc.C) {
	err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertPodSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetPodSpecApplicationOperator(c *gc.C) {
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "elastic-operator", Series: "kubernetes"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, gc.ErrorMatches, "cannot set k8s spec on an operator charm")

	// Nil spec allowed.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PodSpecSuite) TestSetPodSpecApplicationDying(c *gc.C) {
	// create a unit to prevent app from being removed
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.application, state.Dying)

	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, jc.ErrorIsNil)
	s.assertPodSpec(c, s.application.ApplicationTag(), "foo")
}

func (s *PodSpecSuite) TestSetPodSpecApplicationDead(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application})
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.EnsureDead(), jc.ErrorIsNil)
	c.Assert(unit.Remove(), jc.ErrorIsNil)
	assertCleanupCount(c, s.State, 1)
	assertLife(c, s.application, state.Dead)

	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("foo"))
	c.Assert(err, gc.ErrorMatches, "setting pod-spec on dead application gitlab not valid")
	s.assertPodSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestSetPodSpecUpdates(c *gc.C) {
	for _, spec := range []string{"spec0", "spec1"} {
		err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), &spec)
		c.Assert(err, jc.ErrorIsNil)
		s.assertPodSpec(c, s.application.ApplicationTag(), spec)
	}
}

func (s *PodSpecSuite) TestRemoveApplicationRemovesPodSpec(c *gc.C) {
	err := s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// App removal requires cluster resources to be cleared.
	err = s.application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.ClearResources()
	c.Assert(err, jc.ErrorIsNil)
	assertCleanupCount(c, s.State, 2)

	s.assertPodSpecNotFound(c, s.application.ApplicationTag())
}

func (s *PodSpecSuite) TestWatchPodSpec(c *gc.C) {
	w, err := s.Model.WatchPodSpec(s.application.ApplicationTag())
	c.Assert(err, jc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// No spec -> spec set.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// No change to spec but still a change because of incremented counter.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Nil spec also triggers a change because of incremented counter.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Multiple changes coalesced.
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec1"))
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.Model.SetPodSpec(nil, s.application.ApplicationTag(), strPtr("spec2"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *PodSpecSuite) TestWatchRawK8sSpec(c *gc.C) {
	w, err := s.Model.WatchPodSpec(s.application.ApplicationTag())
	c.Assert(err, jc.ErrorIsNil)
	wc := statetesting.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// No spec -> spec set.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// No change to spec but still a change because of incremented counter.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 0"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Nil spec also triggers a change because of incremented counter.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Multiple changes coalesced.
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 1"))
	c.Assert(err, jc.ErrorIsNil)
	// TODO(quiescence): these two changes should be one event.
	wc.AssertOneChange()
	err = s.applySetRawK8sSpecOperation(nil, s.application.ApplicationTag(), strPtr("raw spec 2"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
