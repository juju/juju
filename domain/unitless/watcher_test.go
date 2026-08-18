// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	unitlessservice "github.com/juju/juju/domain/unitless/service"
	unitlessstate "github.com/juju/juju/domain/unitless/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchScriptletApplications(c *tc.C) {
	scriptletCharmID := s.insertCharm(c, true)
	regularCharmID := s.insertCharm(c, false)

	st := unitlessstate.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unitless"),
		loggertesting.WrapCheckLog(c),
	)
	svc := unitlessservice.NewWatchableService(st, factory)
	w, err := svc.WatchScriptletApplications(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		s.insertApplication(c, tc.Must(c, coreapplication.NewUUID), regularCharmID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	scriptletApplicationID := tc.Must(c, coreapplication.NewUUID)
	harness.AddTest(c, func(c *tc.C) {
		s.insertApplication(c, scriptletApplicationID, scriptletCharmID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(scriptletApplicationID.String()))
	})
	harness.AddTest(c, func(c *tc.C) {
		s.setApplicationLife(c, scriptletApplicationID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(scriptletApplicationID.String()))
	})
	harness.AddTest(c, func(c *tc.C) {
		s.setApplicationLife(c, scriptletApplicationID, domainlife.Dead)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(scriptletApplicationID.String()))
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchScriptletApplicationDying(c *tc.C) {
	charmID := s.insertCharm(c, true)
	applicationID := tc.Must(c, coreapplication.NewUUID)
	s.insertApplication(c, applicationID, charmID)

	st := unitlessstate.NewState(s.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unitless-dying"),
		loggertesting.WrapCheckLog(c),
	)
	svc := unitlessservice.NewWatchableService(st, factory)
	w, err := svc.WatchScriptletApplicationDying(c.Context(), applicationID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	harness.AddTest(c, func(c *tc.C) {
		s.setApplicationLife(c, applicationID, domainlife.Dying)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.Run(c, struct{}{})
}

func (s *watcherSuite) insertCharm(c *tc.C, scriptlet bool) corecharm.ID {
	charmID := tc.Must(c, corecharm.NewID)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id)
VALUES (?, ?, 0);
`, charmID.String(), charmID.String())
		if err != nil || !scriptlet {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_scriptlet (charm_uuid, path, content)
VALUES (?, 'hook.star', 'def init(): pass');
`, charmID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return charmID
}

func (s *watcherSuite) insertApplication(
	c *tc.C, applicationID coreapplication.UUID, charmID corecharm.ID,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, 0, ?, '656b4a82-e28c-53d6-a014-f0dd53417eb6');
`, applicationID.String(), applicationID.String(), charmID.String())
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) setApplicationLife(
	c *tc.C, applicationID coreapplication.UUID, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE application
SET life_id = ?
WHERE uuid = ?;
`, life, applicationID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
