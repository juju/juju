// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	"github.com/juju/juju/internal/changestream/testing"
	coretesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) setupUnits(c *gc.C, appName string) {
	logger := coretesting.NewCheckLogger(c)
	st := applicationstate.NewState(s.TxnRunnerFactory(), logger)
	svc := applicationservice.NewService(st, logger, nil)

	unitName := fmt.Sprintf("%s/0", appName)
	err := svc.CreateApplication(context.Background(),
		appName, applicationservice.AddApplicationParams{},
		applicationservice.AddUnitParams{UnitName: &unitName},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func revID(uri *coresecrets.URI, rev int) string {
	return fmt.Sprintf("%s/%d", uri.ID, rev)
}

func (s *watcherSuite) setupServiceAndState(c *gc.C) (*service.WatchableService, *state.State) {
	logger := coretesting.NewCheckLogger(c)
	st := state.NewState(s.TxnRunnerFactory(), logger)
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secret_revision"),
		logger,
	)
	return service.NewWatchableService(
		st, logger,
		factory, nil,
	), st
}

func (s *watcherSuite) TestWatchObsoleteForAppsAndUnitsOwned(c *gc.C) {
	s.setupUnits(c, "mysql")
	s.setupUnits(c, "mediawiki")

	ctx := context.Background()
	svc, st := s.setupServiceAndState(c)

	createNewRevision := func(c *gc.C, uri *coresecrets.URI) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo-new": "bar-new"},
		}
		err := st.UpdateSecret(ctx, uri, sp)
		c.Assert(err, jc.ErrorIsNil)
	}

	sp := secret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri3 := coresecrets.NewURI()
	err = st.CreateCharmApplicationSecret(ctx, 1, uri3, "mediawiki", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri4 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri4, "mediawiki/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	watchAll, err := svc.WatchObsolete(ctx,
		service.CharmSecretOwner{
			Kind: service.ApplicationOwner,
			ID:   "mysql",
		},
		service.CharmSecretOwner{
			Kind: service.UnitOwner,
			ID:   "mysql/0",
		},

		service.CharmSecretOwner{
			Kind: service.ApplicationOwner,
			ID:   "mediawiki",
		},
		service.CharmSecretOwner{
			Kind: service.UnitOwner,
			ID:   "mediawiki/0",
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(watchAll, gc.NotNil)
	defer workertest.CleanKill(c, watchAll)

	wCAll := watchertest.NewStringsWatcherC(c, watchAll)

	// Wait for the initial changes.
	wCAll.AssertChange([]string(nil)...)

	// create revision 2, and obsolete revision 1.
	createNewRevision(c, uri1)
	createNewRevision(c, uri2)
	createNewRevision(c, uri3)
	createNewRevision(c, uri4)

	wCAll.AssertChange(
		revID(uri1, 1),
		revID(uri2, 1),
		revID(uri3, 1),
		revID(uri4, 1),
	)

	// create revision 3, and obsolete revision 2.
	createNewRevision(c, uri1)
	createNewRevision(c, uri2)
	createNewRevision(c, uri3)

	wCAll.AssertChange(
		revID(uri1, 2),
		revID(uri2, 2),
		revID(uri3, 2),
	)

	wCAll.AssertNoChange()
}

func (s *watcherSuite) TestWatchObsoleteForAppsOwned(c *gc.C) {
	s.setupUnits(c, "mysql")

	ctx := context.Background()
	svc, st := s.setupServiceAndState(c)

	createNewRevision := func(c *gc.C, uri *coresecrets.URI) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo-new": "bar-new"},
		}
		err := st.UpdateSecret(ctx, uri, sp)
		c.Assert(err, jc.ErrorIsNil)
	}

	sp := secret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	watchSingleApplicaiton, err := svc.WatchObsolete(ctx,
		service.CharmSecretOwner{
			Kind: service.ApplicationOwner,
			ID:   "mysql",
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(watchSingleApplicaiton, gc.NotNil)
	defer workertest.CleanKill(c, watchSingleApplicaiton)

	wCSingleApplication := watchertest.NewStringsWatcherC(c, watchSingleApplicaiton)

	// Wait for the initial changes.
	wCSingleApplication.AssertChange([]string(nil)...)

	// create revision 2, and obsolete revision 1.
	createNewRevision(c, uri1)
	createNewRevision(c, uri2)

	wCSingleApplication.AssertChange(
		revID(uri1, 1),
	)

	// create revision 3, and obsolete revision 2.
	createNewRevision(c, uri1)
	createNewRevision(c, uri2)

	wCSingleApplication.AssertChange(
		revID(uri1, 2),
	)

	wCSingleApplication.AssertNoChange()
}

func (s *watcherSuite) TestWatchObsoleteForUnitsOwned(c *gc.C) {
	s.setupUnits(c, "mysql")

	ctx := context.Background()
	svc, st := s.setupServiceAndState(c)

	createNewRevision := func(c *gc.C, uri *coresecrets.URI) {
		sp := secret.UpsertSecretParams{
			Data: coresecrets.SecretData{"foo-new": "bar-new"},
		}
		err := st.UpdateSecret(ctx, uri, sp)
		c.Assert(err, jc.ErrorIsNil)
	}

	sp := secret.UpsertSecretParams{
		Data: coresecrets.SecretData{"foo": "bar", "hello": "world"},
	}
	uri1 := coresecrets.NewURI()
	err := st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
	c.Assert(err, jc.ErrorIsNil)

	uri2 := coresecrets.NewURI()
	err = st.CreateCharmUnitSecret(ctx, 1, uri2, "mysql/0", sp)
	c.Assert(err, jc.ErrorIsNil)

	watchSingleUnit, err := svc.WatchObsolete(ctx,
		service.CharmSecretOwner{
			Kind: service.UnitOwner,
			ID:   "mysql/0",
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(watchSingleUnit, gc.NotNil)
	defer workertest.CleanKill(c, watchSingleUnit)

	wCSingleUnit := watchertest.NewStringsWatcherC(c, watchSingleUnit)

	// Wait for the initial changes.
	wCSingleUnit.AssertChange([]string(nil)...)

	// create revision 2, and obsolete revision 1.
	createNewRevision(c, uri1)
	createNewRevision(c, uri2)

	wCSingleUnit.AssertChange(
		revID(uri2, 1),
	)

	// create revision 3, and obsolete revision 2.
	createNewRevision(c, uri1)
	createNewRevision(c, uri2)

	wCSingleUnit.AssertChange(
		revID(uri2, 2),
	)

	wCSingleUnit.AssertNoChange()
}
