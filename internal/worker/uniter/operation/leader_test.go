// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	stdcontext "context"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
)

type LeaderSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&LeaderSuite{})

func (s *LeaderSuite) newFactory(c *tc.C) operation.Factory {
	return operation.NewFactory(operation.FactoryParams{
		Logger: loggertesting.WrapCheckLog(c),
	})
}

func (s *LeaderSuite) TestAcceptLeadership_Prepare_BadState(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.IsNil)
	// accept is only valid in Continue mode, when we're sure nothing is queued
	// or in progress.
	c.Check(err, tc.Equals, operation.ErrCannotAcceptLeadership)
}

func (s *LeaderSuite) TestAcceptLeadership_Prepare_NotLeader(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{Kind: operation.Continue})
	c.Check(newState, tc.IsNil)
	// *execute* is currently just a no-op -- all the meat happens in commit.
	c.Check(err, tc.Equals, operation.ErrSkipExecute)
}

func (s *LeaderSuite) TestAcceptLeadership_Prepare_AlreadyLeader(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{
		Kind:   operation.Continue,
		Leader: true,
	})
	c.Check(newState, tc.IsNil)
	// *execute* is currently just a no-op -- all the meat happens in commit.
	c.Check(err, tc.Equals, operation.ErrSkipExecute)
}

func (s *LeaderSuite) TestAcceptLeadership_Commit_NotLeader_BlankSlate(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(stdcontext.Background(), operation.State{Kind: operation.Continue})
	c.Check(err, tc.Equals, operation.ErrSkipExecute)

	newState, err := op.Commit(stdcontext.Background(), operation.State{
		Kind: operation.Continue,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &operation.State{
		Kind:   operation.RunHook,
		Step:   operation.Queued,
		Hook:   &hook.Info{Kind: hooks.LeaderElected},
		Leader: true,
	})
}

func (s *LeaderSuite) TestAcceptLeadership_Commit_NotLeader_Preserve(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(stdcontext.Background(), operation.State{Kind: operation.Continue})
	c.Check(err, tc.Equals, operation.ErrSkipExecute)

	newState, err := op.Commit(stdcontext.Background(), operation.State{
		Kind:    operation.Continue,
		Started: true,
		Hook:    &hook.Info{Kind: hooks.Install},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(newState, tc.DeepEquals, &operation.State{
		Kind:    operation.RunHook,
		Step:    operation.Queued,
		Hook:    &hook.Info{Kind: hooks.LeaderElected},
		Leader:  true,
		Started: true,
	})
}

func (s *LeaderSuite) TestAcceptLeadership_Commit_AlreadyLeader(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)
	_, err = op.Prepare(stdcontext.Background(), operation.State{Kind: operation.Continue})
	c.Check(err, tc.Equals, operation.ErrSkipExecute)

	newState, err := op.Commit(stdcontext.Background(), operation.State{
		Kind:   operation.Continue,
		Leader: true,
	})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestAcceptLeadership_DoesNotNeedGlobalMachineLock(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewAcceptLeadership()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), tc.IsFalse)
}

func (s *LeaderSuite) TestResignLeadership_Prepare_Leader(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{Leader: true})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestResignLeadership_Prepare_NotLeader(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Prepare(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.Equals, operation.ErrSkipExecute)
}

func (s *LeaderSuite) TestResignLeadership_Execute(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Prepare(stdcontext.Background(), operation.State{Leader: true})
	c.Check(err, tc.ErrorIsNil)

	// Execute is a no-op (which logs that we should run leader-deposed)
	newState, err := op.Execute(stdcontext.Background(), operation.State{})
	c.Check(newState, tc.IsNil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestResignLeadership_Commit_ClearLeader(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Commit(stdcontext.Background(), operation.State{Leader: true})
	c.Check(newState, tc.DeepEquals, &operation.State{})
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestResignLeadership_Commit_PreserveOthers(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)

	newState, err := op.Commit(stdcontext.Background(), overwriteState)
	c.Check(newState, tc.DeepEquals, &overwriteState)
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestResignLeadership_Commit_All(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)

	leaderState := overwriteState
	leaderState.Leader = true
	newState, err := op.Commit(stdcontext.Background(), leaderState)
	c.Check(newState, tc.DeepEquals, &overwriteState)
	c.Check(err, tc.ErrorIsNil)
}

func (s *LeaderSuite) TestResignLeadership_DoesNotNeedGlobalMachineLock(c *tc.C) {
	factory := s.newFactory(c)
	op, err := factory.NewResignLeadership()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.NeedsGlobalMachineLock(), tc.IsFalse)
}
