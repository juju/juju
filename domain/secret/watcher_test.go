// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret_test

import (
	"context"
	"database/sql"
	"fmt"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secret/state"
	"github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/uuid"
	coretesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func setupUnits(c *gc.C, runner database.TxnRunner, appName string) {
	err := runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		applicationUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(context.Background(), `
INSERT INTO application (uuid, name, life_id)
VALUES (?, ?, ?)
`, applicationUUID, appName, life.Alive)
		c.Assert(err, jc.ErrorIsNil)
		netNodeUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(context.Background(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
		c.Assert(err, jc.ErrorIsNil)
		unitUUID := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(context.Background(), `
INSERT INTO unit (uuid, life_id, unit_id, net_node_uuid, application_uuid)
VALUES (?, ?, ?, ?, (SELECT uuid from application WHERE name = ?))
`, unitUUID, life.Alive, fmt.Sprintf("%s/%d", appName, 0), netNodeUUID, appName)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func revID(uri *coresecrets.URI, rev int) string {
	return fmt.Sprintf("%s/%d", uri.ID, rev)
}

func (s *watcherSuite) TestWatchObsolete(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secret_revision")
	logger := coretesting.NewCheckLogger(c)

	ctx := context.Background()

	st := state.NewState(func() (database.TxnRunner, error) { return factory() }, logger)
	db, err := st.DB()
	c.Assert(err, jc.ErrorIsNil)
	setupUnits(c, db, "mysql")
	setupUnits(c, db, "mediawiki")

	svc := service.NewWatchableService(st, logger, domain.NewWatcherFactory(factory, logger), nil)
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
	err = st.CreateCharmApplicationSecret(ctx, 1, uri1, "mysql", sp)
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

	watchSingleApplicaiton, err := svc.WatchObsolete(ctx,
		service.CharmSecretOwner{
			Kind: service.ApplicationOwner,
			ID:   "mysql",
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(watchSingleApplicaiton, gc.NotNil)
	defer workertest.CleanKill(c, watchSingleApplicaiton)

	watchSingleUnit, err := svc.WatchObsolete(ctx,
		service.CharmSecretOwner{
			Kind: service.UnitOwner,
			ID:   "mysql/0",
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(watchSingleUnit, gc.NotNil)
	defer workertest.CleanKill(c, watchSingleUnit)

	wCAll := watchertest.NewStringsWatcherC(c, watchAll)
	wCSingleApplication := watchertest.NewStringsWatcherC(c, watchSingleApplicaiton)
	wCSingleUnit := watchertest.NewStringsWatcherC(c, watchSingleUnit)

	// Wait for the initial changes.
	wCAll.AssertChange([]string(nil)...)
	wCSingleApplication.AssertChange([]string(nil)...)
	wCSingleUnit.AssertChange([]string(nil)...)

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
	wCSingleApplication.AssertChange(
		revID(uri1, 1),
	)
	wCSingleUnit.AssertChange(
		revID(uri2, 1),
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
	wCSingleApplication.AssertChange(
		revID(uri1, 2),
	)
	wCSingleUnit.AssertChange(
		revID(uri2, 2),
	)

	wCAll.AssertNoChange()
	wCSingleApplication.AssertNoChange()
	wCSingleUnit.AssertNoChange()
}
