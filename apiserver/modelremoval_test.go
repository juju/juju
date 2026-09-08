// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/uuid"
)

type modelRemovalSuite struct {
	removals  *modelRemovals
	modelUUID coremodel.UUID
}

func TestModelRemovalSuite(t *testing.T) {
	tc.Run(t, &modelRemovalSuite{})
}

func (s *modelRemovalSuite) SetUpTest(c *tc.C) {
	s.removals = newModelRemovals()
	s.modelUUID = coremodel.UUID(uuid.MustNewUUID().String())
}

// isSignalled reports, without blocking, whether the model has been reported as
// removed.
func isSignalled(removed <-chan struct{}) bool {
	select {
	case <-removed:
		return true
	default:
		return false
	}
}

// TestRemovalReportedToEveryConnection is the point of holding one channel per
// model rather than one watcher per connection: a single removal wakes every
// connection serving that model.
func (s *modelRemovalSuite) TestRemovalReportedToEveryConnection(c *tc.C) {
	first, _ := s.removals.track(s.modelUUID)
	second, _ := s.removals.track(s.modelUUID)

	c.Assert(isSignalled(first), tc.IsFalse)
	c.Assert(isSignalled(second), tc.IsFalse)

	s.removals.notify(s.modelUUID)

	c.Check(isSignalled(first), tc.IsTrue)
	c.Check(isSignalled(second), tc.IsTrue)
}

// TestOtherModelRemovalNotReported verifies that connections are only told
// about their own model's removal.
func (s *modelRemovalSuite) TestOtherModelRemovalNotReported(c *tc.C) {
	removed, _ := s.removals.track(s.modelUUID)

	s.removals.notify(coremodel.UUID(uuid.MustNewUUID().String()))

	c.Check(isSignalled(removed), tc.IsFalse)
}

// TestRemovalOfUnservedModelIgnored verifies that removals of models this API
// server holds no connections for - most of them, on most API servers - are
// simply dropped.
func (s *modelRemovalSuite) TestRemovalOfUnservedModelIgnored(c *tc.C) {
	s.removals.notify(s.modelUUID)

	c.Check(s.removals.served, tc.HasLen, 0)
}

// TestModelForgottenWithLastConnection verifies that a model is tracked only
// while it is being served, so that the API server does not accumulate an entry
// for every model it has ever been asked about.
func (s *modelRemovalSuite) TestModelForgottenWithLastConnection(c *tc.C) {
	_, firstDone := s.removals.track(s.modelUUID)
	_, secondDone := s.removals.track(s.modelUUID)

	firstDone()
	c.Assert(s.removals.served, tc.HasLen, 1)

	secondDone()
	c.Check(s.removals.served, tc.HasLen, 0)
}

// TestModelForgottenWhenRemoved verifies that a removed model is forgotten as
// it is reported, so that a connection arriving afterwards to be redirected is
// served rather than closed straight away.
func (s *modelRemovalSuite) TestModelForgottenWhenRemoved(c *tc.C) {
	_, done := s.removals.track(s.modelUUID)

	s.removals.notify(s.modelUUID)
	c.Assert(s.removals.served, tc.HasLen, 0)

	redirected, redirectedDone := s.removals.track(s.modelUUID)
	c.Check(isSignalled(redirected), tc.IsFalse)

	// The connection that was open when the model was removed must not take the
	// later connection's model with it as it goes.
	done()
	c.Check(s.removals.served, tc.HasLen, 1)

	redirectedDone()
	c.Check(s.removals.served, tc.HasLen, 0)
}
