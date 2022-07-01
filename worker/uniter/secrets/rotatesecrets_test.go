// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9/hooks"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/life"
	"github.com/juju/juju/v3/worker/uniter/hook"
	"github.com/juju/juju/v3/worker/uniter/operation"
	"github.com/juju/juju/v3/worker/uniter/operation/mocks"
	"github.com/juju/juju/v3/worker/uniter/remotestate"
	"github.com/juju/juju/v3/worker/uniter/resolver"
	"github.com/juju/juju/v3/worker/uniter/secrets"
)

type rotateSecretsSuite struct {
	remoteState   remotestate.Snapshot
	mockCallbacks *mocks.MockCallbacks
	opFactory     operation.Factory
	resolver      resolver.Resolver
	rotatedSecret func(string)
}

var _ = gc.Suite(&rotateSecretsSuite{})

func (s *rotateSecretsSuite) SetUpTest(_ *gc.C) {
	s.remoteState = remotestate.Snapshot{
		Leader: true,
		Life:   life.Alive,
	}

	s.rotatedSecret = nil
	s.resolver = secrets.NewSecretsResolver(func(url string) {
		if s.rotatedSecret != nil {
			s.rotatedSecret(url)
		}
	},
	)
}

func (s *rotateSecretsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctlr := gomock.NewController(c)
	s.mockCallbacks = mocks.NewMockCallbacks(ctlr)
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Callbacks: s.mockCallbacks,
		Logger:    loggo.GetLogger("test"),
	})
	return ctlr
}

func (s *rotateSecretsSuite) TestNextOpNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	s.remoteState.SecretRotations = []string{"secret://app/mariadb/password"}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *rotateSecretsSuite) TestNextOpNotLeader(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.Leader = false
	s.remoteState.SecretRotations = []string{"secret://app/mariadb/password"}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *rotateSecretsSuite) TestNextOpNotAlive(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.Life = life.Dying
	s.remoteState.SecretRotations = []string{"secret://app/mariadb/password"}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *rotateSecretsSuite) TestNextOpNotReady(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Upgrade,
			Installed: true,
		},
	}
	s.remoteState.SecretRotations = []string{"secret://app/mariadb/password"}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *rotateSecretsSuite) TestNextOpRotate(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.SecretRotations = []string{"secret://app/mariadb/password"}
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run secret-rotate (secret://app/mariadb/password) hook")
}

func (s *rotateSecretsSuite) TestRotateCommit(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	localState := resolver.LocalState{
		State: operation.State{
			Kind:      operation.Continue,
			Installed: true,
		},
	}
	s.remoteState.SecretRotations = []string{"secret://app/mariadb/password"}
	var rotatedURL string
	s.rotatedSecret = func(url string) {
		rotatedURL = url
	}
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)

	hi := hook.Info{
		Kind:      hooks.SecretRotate,
		SecretURL: "secret://app/mariadb/password",
	}
	s.mockCallbacks.EXPECT().CommitHook(hi).Return(nil)
	now := time.Now()
	s.mockCallbacks.EXPECT().SetSecretRotated("secret://app/mariadb/password", gomock.Any()).DoAndReturn(
		func(url string, when time.Time) error {
			c.Assert(url, gc.Equals, "secret://app/mariadb/password")
			c.Assert(when.After(now), jc.IsTrue)
			return nil
		},
	)
	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rotatedURL, gc.Equals, "secret://app/mariadb/password")
}
