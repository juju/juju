// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/life"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	operationmocks "github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	runnermocks "github.com/juju/juju/internal/worker/uniter/runner/mocks"
	"github.com/juju/juju/internal/worker/uniter/secrets"
	"github.com/juju/juju/internal/worker/uniter/secrets/mocks"
)

type triggerSecretsSuite struct {
	remoteState     remotestate.Snapshot
	mockCallbacks   *operationmocks.MockCallbacks
	mockFactory     *runnermocks.MockFactory
	mockRunner      *runnermocks.MockRunner
	mockContext     *runnermocks.MockContext
	mockTracker     *mocks.MockSecretStateTracker
	opFactory       operation.Factory
	resolver        resolver.Resolver
	rotatedSecret   func(string)
	expiredRevision func(string)
	deletedSecrets  func([]string)
}

func TestTriggerSecretsSuite(t *stdtesting.T) { tc.Run(t, &triggerSecretsSuite{}) }
func (s *triggerSecretsSuite) SetUpTest(c *tc.C) {
	s.remoteState = remotestate.Snapshot{
		Life: life.Alive,
	}

	s.rotatedSecret = nil
	logger := loggertesting.WrapCheckLog(c)
	s.resolver = secrets.NewSecretsResolver(logger, s.mockTracker, func(url string) {
		if s.rotatedSecret != nil {
			s.rotatedSecret(url)
		}
	}, func(rev string) {
		if s.expiredRevision != nil {
			s.expiredRevision(rev)
		}
	}, func(uris []string) {
		if s.deletedSecrets != nil {
			s.deletedSecrets(uris)
		}
	},
	)
}

func (s *triggerSecretsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockCallbacks = operationmocks.NewMockCallbacks(ctlr)
	s.mockFactory = runnermocks.NewMockFactory(ctlr)
	s.mockRunner = runnermocks.NewMockRunner(ctlr)
	s.mockContext = runnermocks.NewMockContext(ctlr)
	s.mockTracker = mocks.NewMockSecretStateTracker(ctlr)
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Callbacks:     s.mockCallbacks,
		RunnerFactory: s.mockFactory,
		Logger:        loggertesting.WrapCheckLog(c),
	})
	return ctlr
}

func (s *triggerSecretsSuite) TestNextOpNotInstalled(c *tc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	s.remoteState.SecretRotations = []string{"secret:9m4e2mr0ui3e8a215n4g"}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *triggerSecretsSuite) TestNextOpNotAlive(c *tc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.Life = life.Dying
	s.remoteState.SecretRotations = []string{"secret:9m4e2mr0ui3e8a215n4g"}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *triggerSecretsSuite) TestNextOpNotReady(c *tc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
		},
	}
	s.remoteState.SecretRotations = []string{"secret:9m4e2mr0ui3e8a215n4g"}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *triggerSecretsSuite) TestNextOpRotate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.SecretRotations = []string{"secret:9m4e2mr0ui3e8a215n4g"}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-rotate (secret:9m4e2mr0ui3e8a215n4g) hook")
}

func (s *triggerSecretsSuite) TestRotateCommit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	uri := coresecrets.NewURI()
	s.remoteState.SecretRotations = []string{uri.String()}
	var rotatedURI string
	s.rotatedSecret = func(uri string) {
		rotatedURI = uri
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)

	hi := hook.Info{
		Kind:      hooks.SecretRotate,
		SecretURI: uri.String(),
	}
	s.mockCallbacks.EXPECT().PrepareHook(gomock.Any(), hi).Return("", nil)
	s.mockFactory.EXPECT().NewHookRunner(gomock.Any(), hi).Return(s.mockRunner, nil)
	s.mockRunner.EXPECT().Context().Return(s.mockContext).AnyTimes()
	s.mockContext.EXPECT().Prepare(gomock.Any()).Return(nil)
	s.mockContext.EXPECT().SecretMetadata().Return(map[string]jujuc.SecretMetadata{
		uri.ID: {
			LatestRevision: 666,
		},
	}, nil)
	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	s.mockCallbacks.EXPECT().CommitHook(gomock.Any(), hi).Return(nil)
	s.mockCallbacks.EXPECT().SetSecretRotated(gomock.Any(), uri.String(), 666).Return(nil)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rotatedURI, tc.Equals, uri.String())
}

func (s *triggerSecretsSuite) TestNextOpExpire(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ExpiredSecretRevisions = []string{"secret:9m4e2mr0ui3e8a215n4g/666"}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-expired (secret:9m4e2mr0ui3e8a215n4g/666) hook")
}

func (s *triggerSecretsSuite) TestExpireCommit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	uri := coresecrets.NewURI()
	s.remoteState.ExpiredSecretRevisions = []string{uri.String() + "/666"}
	var expiredRevision string
	s.expiredRevision = func(rev string) {
		expiredRevision = rev
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)

	hi := hook.Info{
		Kind:           hooks.SecretExpired,
		SecretURI:      uri.String(),
		SecretRevision: 666,
	}
	s.mockCallbacks.EXPECT().PrepareHook(gomock.Any(), hi).Return("", nil)
	s.mockFactory.EXPECT().NewHookRunner(gomock.Any(), hi).Return(s.mockRunner, nil)
	s.mockRunner.EXPECT().Context().Return(s.mockContext).AnyTimes()
	s.mockContext.EXPECT().Prepare(gomock.Any()).Return(nil)
	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)

	s.mockCallbacks.EXPECT().CommitHook(gomock.Any(), hi).Return(nil)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(expiredRevision, tc.Equals, uri.String()+"/666")
}

type changeSecretsSuite struct {
	remoteState remotestate.Snapshot
	opFactory   operation.Factory
	tracker     *mocks.MockSecretStateTracker
	resolver    resolver.Resolver
}

func TestChangeSecretsSuite(t *stdtesting.T) { tc.Run(t, &changeSecretsSuite{}) }
func (s *changeSecretsSuite) SetUpTest(_ *tc.C) {
	s.remoteState = remotestate.Snapshot{
		Life: life.Alive,
	}
}

func (s *changeSecretsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	logger := loggertesting.WrapCheckLog(c)
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Logger: logger,
	})
	s.tracker = mocks.NewMockSecretStateTracker(ctlr)
	s.resolver = secrets.NewSecretsResolver(logger, s.tracker, nil, nil, nil)
	return ctlr
}

func (s *changeSecretsSuite) TestNextOpNotInstalled(c *tc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	s.remoteState.ConsumedSecretInfo = map[string]coresecrets.SecretRevisionInfo{
		"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 666},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *changeSecretsSuite) TestNextOpNoneExisting(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.tracker.EXPECT().ConsumedSecretRevision("secret:9m4e2mr0ui3e8a215n4g").Return(0)

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ConsumedSecretInfo = map[string]coresecrets.SecretRevisionInfo{
		"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 666},
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-changed (secret:9m4e2mr0ui3e8a215n4g) hook")
}

func (s *changeSecretsSuite) TestNextOpUpdatedRevision(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.tracker.EXPECT().ConsumedSecretRevision("secret:9m4e2mr0ui3e8a215n4g").Return(667)

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ConsumedSecretInfo = map[string]coresecrets.SecretRevisionInfo{
		"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 666},
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-changed (secret:9m4e2mr0ui3e8a215n4g) hook")
}

func (s *changeSecretsSuite) TestNextOpNone(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.tracker.EXPECT().ConsumedSecretRevision("secret:9m4e2mr0ui3e8a215n4g").Return(666)

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ConsumedSecretInfo = map[string]coresecrets.SecretRevisionInfo{
		"secret:9m4e2mr0ui3e8a215n4g": {LatestRevision: 666},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

type removeSecretSuite struct {
	remoteState remotestate.Snapshot
	opFactory   operation.Factory
	tracker     *mocks.MockSecretStateTracker
	resolver    resolver.Resolver
}

func TestRemoveSecretSuite(t *stdtesting.T) { tc.Run(t, &removeSecretSuite{}) }
func (s *removeSecretSuite) SetUpTest(_ *tc.C) {
	s.remoteState = remotestate.Snapshot{
		Life: life.Alive,
	}
}

func (s *removeSecretSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	logger := loggertesting.WrapCheckLog(c)
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Logger: logger,
	})
	s.tracker = mocks.NewMockSecretStateTracker(ctlr)
	s.resolver = secrets.NewSecretsResolver(logger, s.tracker, nil, nil, nil)
	return ctlr
}

func (s *removeSecretSuite) TestNextOpNotInstalled(c *tc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	s.remoteState.ObsoleteSecretRevisions = map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666, 668},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *removeSecretSuite) TestNextOpNoneExisting(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.tracker.EXPECT().SecretObsoleteRevisions("secret:9m4e2mr0ui3e8a215n4g").Return(nil)

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ObsoleteSecretRevisions = map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666, 668},
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-remove (secret:9m4e2mr0ui3e8a215n4g/666) hook")
}

func (s *removeSecretSuite) TestNextOpNextRevision(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.tracker.EXPECT().SecretObsoleteRevisions("secret:9m4e2mr0ui3e8a215n4g").Return([]int{666})

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ObsoleteSecretRevisions = map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666, 668},
	}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run secret-remove (secret:9m4e2mr0ui3e8a215n4g/668) hook")
}

func (s *removeSecretSuite) TestNextOpNone(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.tracker.EXPECT().SecretObsoleteRevisions("secret:9m4e2mr0ui3e8a215n4g").Return([]int{666, 668})

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.ObsoleteSecretRevisions = map[string][]int{
		"secret:9m4e2mr0ui3e8a215n4g": {666, 668},
	}
	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

type secretDeletedSuite struct {
	remoteState   remotestate.Snapshot
	opFactory     operation.Factory
	mockTracker   *mocks.MockSecretStateTracker
	mockCallbacks *operationmocks.MockCallbacks
	resolver      resolver.Resolver

	deleted []string
}

func TestSecretDeletedSuite(t *stdtesting.T) { tc.Run(t, &secretDeletedSuite{}) }
func (s *secretDeletedSuite) SetUpTest(_ *tc.C) {
	s.remoteState = remotestate.Snapshot{
		Life: life.Alive,
	}
}

func (s *secretDeletedSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	logger := loggertesting.WrapCheckLog(c)
	s.resolver = secrets.NewSecretsResolver(logger, s.mockTracker, nil, nil, func(uris []string) {
		s.deleted = uris
	})
	s.mockCallbacks = operationmocks.NewMockCallbacks(ctlr)
	s.mockTracker = mocks.NewMockSecretStateTracker(ctlr)
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Callbacks: s.mockCallbacks,
		Logger:    loggertesting.WrapCheckLog(c),
	})
	return ctlr
}

func (s *secretDeletedSuite) TestNextOpNotInstalled(c *tc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	s.remoteState.DeletedSecrets = []string{"secret:9m4e2mr0ui3e8a215n4g"}

	_, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.Equals, resolver.ErrNoOperation)
}

func (s *secretDeletedSuite) TestNextOp(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.DeletedSecrets = []string{"secret:9m4e2mr0ui3e8a215n4g"}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "process removed secrets: [secret:9m4e2mr0ui3e8a215n4g]")
}

func (s *secretDeletedSuite) TestCommit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.DeletedSecrets = []string{"secret:9m4e2mr0ui3e8a215n4g"}
	op, err := s.resolver.NextOp(c.Context(), localState, s.remoteState, s.opFactory)
	c.Assert(err, tc.ErrorIsNil)

	_, err = op.Prepare(c.Context(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)
	_, err = op.Execute(c.Context(), operation.State{})
	c.Assert(err, tc.Equals, operation.ErrSkipExecute)

	s.mockCallbacks.EXPECT().SecretsRemoved(gomock.Any(), []string{"secret:9m4e2mr0ui3e8a215n4g"}).Return(nil)

	_, err = op.Commit(c.Context(), operation.State{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.deleted, tc.DeepEquals, []string{"secret:9m4e2mr0ui3e8a215n4g"})
}
