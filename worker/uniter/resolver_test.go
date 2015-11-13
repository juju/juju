// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/worker/uniter"
	uniteractions "github.com/juju/juju/worker/uniter/actions"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/leadership"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/storage"
)

type resolverSuite struct {
	charmURL    *charm.URL
	remoteState remotestate.Snapshot
	opFactory   operation.Factory
	resolver    resolver.Resolver
}

var _ = gc.Suite(&resolverSuite{})

func (s *resolverSuite) SetUpTest(c *gc.C) {
	s.charmURL = charm.MustParseURL("cs:precise/mysql-2")
	s.remoteState = remotestate.Snapshot{
		CharmURL: s.charmURL,
	}
	s.opFactory = operation.NewFactory(operation.FactoryParams{})

	attachments, err := storage.NewAttachments(&dummyStorageAccessor{}, names.NewUnitTag("u/0"), c.MkDir(), nil)
	c.Assert(err, jc.ErrorIsNil)

	s.resolver = uniter.NewUniterResolver(uniter.ResolverConfig{
		ClearResolved:   func() error { return errors.New("unexpected resolved") },
		ReportHookError: func(_ hook.Info) error { return errors.New("unexpected report hook error") },
		FixDeployer:     func() error { return nil },
		Leadership:      leadership.NewResolver(),
		Actions:         uniteractions.NewResolver(),
		Relations:       relation.NewRelationsResolver(&dummyRelations{}),
		Storage:         storage.NewResolver(attachments),
		Commands:        nopResolver{},
	})
}

// TestStartedNotInstalled tests whether the Started flag overrides the
// Installed flag being unset, in the event of an unexpected inconsistency in
// local state.
func (s *resolverSuite) TestStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

// TestNotStartedNotInstalled tests whether the next operation for an
// uninstalled local state is an install hook operation.
func (s *resolverSuite) TestNotStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run install hook")
}
