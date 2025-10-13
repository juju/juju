// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/life"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type modelSecretsSuite struct {
	baseSuite
}

func TestModelSecretsSuite(t *testing.T) {
	tc.Run(t, &modelSecretsSuite{})
}

func getRevUUID(c *tc.C, db *sql.DB, uri *coresecrets.URI, rev int) string {
	var uuid string
	row := db.QueryRowContext(c.Context(), `
SELECT uuid
FROM secret_revision
WHERE secret_id = ? AND revision = ?
`, uri.ID, rev)
	err := row.Scan(&uuid)
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

func (s *modelSecretsSuite) getObsolete(c *tc.C, uri *coresecrets.URI, rev int) (bool, bool) {
	var obsolete, pendingDelete bool
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT obsolete, pending_delete
FROM secret_revision_obsolete sro
INNER JOIN secret_revision sr ON sro.revision_uuid = sr.uuid
WHERE sr.secret_id = ? AND sr.revision = ?`, uri.ID, rev)
		err := row.Scan(&obsolete, &pendingDelete)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	return obsolete, pendingDelete
}

func (s *modelSecretsSuite) setupRemoteApp(c *tc.C, appName string) string {
	appUUID := internaluuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		charmUUID := internaluuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id)
VALUES (?, ?, 0);
`, charmUUID, appName)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?);
		`, charmUUID, appName)
		if err != nil {
			return errors.Capture(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, appUUID, charmUUID, appName, life.Alive, network.AlphaSpaceId)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return appUUID
}

func (s *modelSecretsSuite) createSecret(c *tc.C, uri *coresecrets.URI, content map[string]string) {
	c.Assert(content, tc.Not(tc.HasLen), 0)

	now := time.Now().UTC()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO secret (id) VALUES (?)`, uri.ID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO secret_metadata (secret_id, version, rotate_policy_id, auto_prune, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?)`,
			uri.ID, 1, 0, false, now, now,
		)
		if err != nil {
			return err
		}
		revisionUUID := internaluuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO secret_revision (uuid, secret_id, revision, create_time) VALUES (?, ?, ?, ?)`,
			revisionUUID, uri.ID, 1, now,
		)
		if err != nil {
			return err
		}
		for k, v := range content {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO secret_content (revision_uuid, name, content) VALUES (?, ?, ?)`,
				revisionUUID, k, v,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretsSuite) addRevision(c *tc.C, uri *coresecrets.URI, content map[string]string) {
	c.Assert(content, tc.Not(tc.HasLen), 0)

	now := time.Now().UTC()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		revisionUUID := internaluuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx,
			`
INSERT INTO secret_revision (uuid, secret_id, revision, create_time) 
VALUES (?, ?, (SELECT MAX(revision)+1 FROM secret_revision WHERE secret_id=?), ?)`,
			revisionUUID, uri.ID, uri.ID, now,
		)
		if err != nil {
			return err
		}
		for k, v := range content {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO secret_content (revision_uuid, name, content) VALUES (?, ?, ?)`,
				revisionUUID, k, v,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretsSuite) prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c *tc.C) (string, *coresecrets.URI, *coresecrets.URI) {
	ctx := c.Context()
	saveRemoteConsumer := func(uri *coresecrets.URI, revision int, consumerID string) {
		consumer := coresecrets.SecretConsumerMetadata{
			CurrentRevision: revision,
		}
		err := s.state.SaveSecretRemoteConsumer(ctx, uri, consumerID, consumer)
		c.Assert(err, tc.ErrorIsNil)
	}

	uri1 := coresecrets.NewURI()
	s.createSecret(c, uri1, map[string]string{"foo": "bar", "hello": "world"})
	uri1.SourceUUID = s.ModelUUID()

	uri2 := coresecrets.NewURI()
	s.createSecret(c, uri2, map[string]string{"foo": "bar", "hello": "world"})
	uri2.SourceUUID = s.ModelUUID()

	appUUID := s.setupRemoteApp(c, "mediawiki")

	// The consumed revision 1.
	saveRemoteConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveRemoteConsumer(uri2, 1, "mediawiki/0")

	// create revision 2.
	s.addRevision(c, uri1, map[string]string{"foo": "bar2"})

	err := s.state.UpdateRemoteSecretRevision(ctx, uri1, 2)
	c.Assert(err, tc.ErrorIsNil)
	return appUUID, uri1, uri2
}

func (s *modelSecretsSuite) TestInitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(c *tc.C) {
	appUUID, uri1, _ := s.prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c)

	tableName, f := s.state.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(appUUID)
	c.Assert(tableName, tc.Equals, "secret_revision")
	result, err := f(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{
		getRevUUID(c, s.DB(), uri1, 2),
	})
}

func (s *modelSecretsSuite) TestGetRemoteConsumedSecretURIsWithChangesFromOfferingSide(c *tc.C) {
	ctx := c.Context()
	appUUID, uri1, uri2 := s.prepareWatchForRemoteConsumedSecretsChangesFromOfferingSide(c)

	result, err := s.state.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx, appUUID,
		getRevUUID(c, s.DB(), uri1, 1),
		getRevUUID(c, s.DB(), uri1, 2),
		getRevUUID(c, s.DB(), uri2, 1),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result, tc.SameContents, []string{
		uri1.String(),
	})
}

func (s *modelSecretsSuite) TestSaveSecretRemoteConsumer(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"})

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}

	ctx := c.Context()
	err := s.state.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", *consumer)
	c.Assert(err, tc.ErrorIsNil)

	got, latest, err := s.state.GetSecretRemoteConsumer(ctx, uri, "remote-app/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
	c.Assert(latest, tc.Equals, 1)
}

func (s *modelSecretsSuite) TestSaveSecretRemoteConsumerMarksObsolete(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"})
	s.addRevision(c, uri, map[string]string{"foo": "bar2"})

	consumer := coresecrets.SecretConsumerMetadata{
		CurrentRevision: 1,
	}

	ctx := c.Context()
	err := s.state.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	consumer.CurrentRevision = 2
	err = s.state.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	// Revision 1 is obsolete.
	obsolete, pendingDelete := s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)

	// But not revision 2.
	obsolete, pendingDelete = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsFalse)
	c.Check(pendingDelete, tc.IsFalse)
}

func (s *modelSecretsSuite) TestSaveSecretRemoteConsumerSecretNotExists(c *tc.C) {
	uri := coresecrets.NewURI().WithSource(s.ModelUUID())
	ctx := c.Context()
	consumer := coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}

	err := s.state.SaveSecretRemoteConsumer(ctx, uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *modelSecretsSuite) TestGetSecretRemoteConsumerFirstTime(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"})

	ctx := c.Context()
	_, latest, err := s.state.GetSecretRemoteConsumer(ctx, uri, "remote-app/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretConsumerNotFound)
	c.Assert(latest, tc.Equals, 1)
}

func (s *modelSecretsSuite) TestGetSecretRemoteConsumerSecretNotExists(c *tc.C) {
	uri := coresecrets.NewURI()

	_, _, err := s.state.GetSecretRemoteConsumer(c.Context(), uri, "remite-app/0")
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *modelSecretsSuite) TestUpdateRemoteSecretRevision(c *tc.C) {
	uri := coresecrets.NewURI()

	getLatest := func() int {
		var got int
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			row := tx.QueryRowContext(ctx, `
			SELECT latest_revision FROM secret_reference WHERE secret_id = ?
		`, uri.ID)
			if err := row.Scan(&got); err != nil {
				return err
			}
			return row.Err()
		})
		c.Assert(err, tc.ErrorIsNil)
		return got
	}

	err := s.state.UpdateRemoteSecretRevision(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
	got := getLatest()
	c.Assert(got, tc.Equals, 666)
	err = s.state.UpdateRemoteSecretRevision(c.Context(), uri, 667)
	c.Assert(err, tc.ErrorIsNil)
	got = getLatest()
	c.Assert(got, tc.Equals, 667)
}
