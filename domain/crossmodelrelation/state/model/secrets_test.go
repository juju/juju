// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreoffer "github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/life"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
	coretesting "github.com/juju/juju/internal/testing"
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

func (s *modelSecretsSuite) createSecret(c *tc.C, uri *coresecrets.URI, content map[string]string, valueRef *coresecrets.ValueRef) {
	if valueRef == nil {
		c.Assert(content, tc.Not(tc.HasLen), 0)
	}

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
			`INSERT INTO secret_revision (uuid, secret_id, revision, create_time, update_time) VALUES (?, ?, ?, ?, ?)`,
			revisionUUID, uri.ID, 1, now, now,
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

		if valueRef == nil {
			return nil
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO secret_value_ref (revision_uuid, backend_uuid, revision_id) VALUES (?, ?, ?)`,
			revisionUUID, valueRef.BackendID, valueRef.RevisionID,
		)
		return err
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
INSERT INTO secret_revision (uuid, secret_id, revision, create_time, update_time) 
VALUES (?, ?, (SELECT MAX(revision)+1 FROM secret_revision WHERE secret_id=?), ?, ?)`,
			revisionUUID, uri.ID, uri.ID, now, now,
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
	s.createSecret(c, uri1, map[string]string{"foo": "bar", "hello": "world"}, nil)
	uri1.SourceUUID = s.ModelUUID()

	uri2 := coresecrets.NewURI()
	s.createSecret(c, uri2, map[string]string{"foo": "bar", "hello": "world"}, nil)
	uri2.SourceUUID = s.ModelUUID()

	_, _, realApplicationUUID, syntheticApplicationUUID, _ := s.setupRemoteApplicationConsumer(c)

	// The consumed revision 1.
	saveRemoteConsumer(uri1, 1, syntheticApplicationUUID+"/0")
	// The consumed revision 1.
	saveRemoteConsumer(uri2, 1, syntheticApplicationUUID+"/0")

	// create revision 2.
	s.addRevision(c, uri1, map[string]string{"foo": "bar2"})

	err := s.state.UpdateRemoteSecretRevision(ctx, uri1, 2, syntheticApplicationUUID)
	c.Assert(err, tc.ErrorIsNil)

	return realApplicationUUID, uri1, uri2
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

// addOfferedApplication adds an application, which can be offered, with
// the given name. It returns the application UUID.
func (s *modelSecretsSuite) addOfferedApplication(c *tc.C, name string) string {
	return s.addApplication(c, s.addCharm(c), name).String()
}

// addRemoteSecretConsumerApplication adds a synthetic application with the
// given name, representing a consuming application connected to the
// offered application through its own offer connection.
func (s *modelSecretsSuite) addRemoteSecretConsumerApplication(c *tc.C, offeredAppUUID, name string) {
	offerUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)
	synthAppUUID := s.addApplication(c, s.addCharm(c), name)
	relUUID := s.addRelation(c)

	s.query(c, `
INSERT INTO offer_connection (uuid, offer_uuid, remote_relation_uuid, username)
VALUES (?, ?, ?, 'bob')`, synthAppUUID, offerUUID, relUUID)
	s.query(c, `
INSERT INTO application_remote_consumer (
    offer_connection_uuid,
    offerer_application_uuid,
    consumer_application_uuid,
    consumer_model_uuid,
    life_id)
VALUES (?, ?, ?, ?, 0)`,
		synthAppUUID,
		offeredAppUUID,
		tc.Must(c, coreapplication.NewUUID),
		tc.Must(c, coremodel.NewUUID),
	)
}

// addSecretWithRevisions adds a secret with the given number of revisions.
func (s *modelSecretsSuite) addSecretWithRevisions(c *tc.C, revisions int) *coresecrets.URI {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)
	for i := 1; i < revisions; i++ {
		s.addRevision(c, uri, map[string]string{"foo": "bar"})
	}
	uri.SourceUUID = s.ModelUUID()
	return uri
}

// saveRemoteSecretConsumer records the revision of the secret currently
// consumed by the given remote unit.
func (s *modelSecretsSuite) saveRemoteSecretConsumer(c *tc.C, uri *coresecrets.URI, unitName string, revision int) {
	err := s.state.SaveSecretRemoteConsumer(c.Context(), uri, unitName, coresecrets.SecretConsumerMetadata{
		CurrentRevision: revision,
	})
	c.Assert(err, tc.ErrorIsNil)
}

// assertRemoteConsumedSecretsChanges checks the revisions returned by the
// initial watch statement and the secrets returned when no revisions are
// filtered and when filtering by the initial revisions.
func (s *modelSecretsSuite) assertRemoteConsumedSecretsChanges(
	c *tc.C, offeredAppUUID string, expectedRevUUIDs []string, expectedURIs []string,
) {
	_, f := s.state.InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(offeredAppUUID)
	revUUIDs, err := f(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revUUIDs, tc.SameContents, expectedRevUUIDs)

	uris, err := s.state.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(c.Context(), offeredAppUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uris, tc.SameContents, expectedURIs)

	if len(revUUIDs) == 0 {
		return
	}
	uris, err = s.state.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(c.Context(), offeredAppUUID, revUUIDs...)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uris, tc.SameContents, expectedURIs)
}

func (s *modelSecretsSuite) TestRemoteConsumedSecretsChangesNoConsumers(c *tc.C) {
	offeredAppUUID := s.addOfferedApplication(c, "offered")
	s.addRemoteSecretConsumerApplication(c, offeredAppUUID, "remote-consumer")
	uri := s.addSecretWithRevisions(c, 2)

	s.assertRemoteConsumedSecretsChanges(c, offeredAppUUID, nil, nil)

	uris, err := s.state.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(
		c.Context(), offeredAppUUID, getRevUUID(c, s.DB(), uri, 2))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uris, tc.HasLen, 0)
}

func (s *modelSecretsSuite) TestRemoteConsumedSecretsChangesSingleConsumer(c *tc.C) {
	offeredAppUUID := s.addOfferedApplication(c, "offered")
	s.addRemoteSecretConsumerApplication(c, offeredAppUUID, "remote-consumer")

	current := s.addSecretWithRevisions(c, 2)
	s.saveRemoteSecretConsumer(c, current, "remote-consumer/0", 2)
	behind := s.addSecretWithRevisions(c, 2)
	s.saveRemoteSecretConsumer(c, behind, "remote-consumer/0", 1)

	s.assertRemoteConsumedSecretsChanges(c, offeredAppUUID,
		[]string{getRevUUID(c, s.DB(), behind, 2)},
		[]string{behind.String()},
	)
}

// TestRemoteConsumedSecretsChangesMixedUnitRevisions checks that a secret
// is reported when any consuming unit is behind the latest revision, even
// if another unit already consumes the latest revision. The arrangements
// vary which unit is behind and the order in which consumers are recorded,
// so that the result cannot depend on which unit row is evaluated first.
func (s *modelSecretsSuite) TestRemoteConsumedSecretsChangesMixedUnitRevisions(c *tc.C) {
	offeredAppUUID := s.addOfferedApplication(c, "offered")
	s.addRemoteSecretConsumerApplication(c, offeredAppUUID, "remote-consumer")

	type consumer struct {
		unitName string
		revision int
	}
	arrangements := [][]consumer{
		{{"remote-consumer/0", 2}, {"remote-consumer/1", 1}},
		{{"remote-consumer/1", 1}, {"remote-consumer/0", 2}},
		{{"remote-consumer/0", 1}, {"remote-consumer/1", 2}},
		{{"remote-consumer/1", 2}, {"remote-consumer/0", 1}},
	}

	var (
		expectedRevUUIDs []string
		expectedURIs     []string
	)
	for _, consumers := range arrangements {
		uri := s.addSecretWithRevisions(c, 2)
		for _, consumer := range consumers {
			s.saveRemoteSecretConsumer(c, uri, consumer.unitName, consumer.revision)
		}
		expectedRevUUIDs = append(expectedRevUUIDs, getRevUUID(c, s.DB(), uri, 2))
		expectedURIs = append(expectedURIs, uri.String())
	}

	s.assertRemoteConsumedSecretsChanges(c, offeredAppUUID, expectedRevUUIDs, expectedURIs)
}

// TestRemoteConsumedSecretsChangesLatestRevisionOnce checks that the
// initial watch statement returns only the latest revision of each secret,
// once, however many consuming units are behind.
func (s *modelSecretsSuite) TestRemoteConsumedSecretsChangesLatestRevisionOnce(c *tc.C) {
	offeredAppUUID := s.addOfferedApplication(c, "offered")
	s.addRemoteSecretConsumerApplication(c, offeredAppUUID, "remote-consumer")
	s.addRemoteSecretConsumerApplication(c, offeredAppUUID, "remote-other")

	allBehind := s.addSecretWithRevisions(c, 3)
	s.saveRemoteSecretConsumer(c, allBehind, "remote-consumer/0", 1)
	s.saveRemoteSecretConsumer(c, allBehind, "remote-consumer/1", 2)
	s.saveRemoteSecretConsumer(c, allBehind, "remote-other/0", 1)

	oneBehind := s.addSecretWithRevisions(c, 3)
	s.saveRemoteSecretConsumer(c, oneBehind, "remote-consumer/0", 3)
	s.saveRemoteSecretConsumer(c, oneBehind, "remote-other/0", 2)

	allCurrent := s.addSecretWithRevisions(c, 3)
	s.saveRemoteSecretConsumer(c, allCurrent, "remote-consumer/0", 3)
	s.saveRemoteSecretConsumer(c, allCurrent, "remote-other/0", 3)

	s.assertRemoteConsumedSecretsChanges(c, offeredAppUUID,
		[]string{
			getRevUUID(c, s.DB(), allBehind, 3),
			getRevUUID(c, s.DB(), oneBehind, 3),
		},
		[]string{allBehind.String(), oneBehind.String()},
	)
}

// TestRemoteConsumedSecretsChangesFilteredRevisions checks that, when
// filtering by revisions, a secret is reported only if a filtered revision
// is newer than the revision consumed by any consuming unit.
func (s *modelSecretsSuite) TestRemoteConsumedSecretsChangesFilteredRevisions(c *tc.C) {
	offeredAppUUID := s.addOfferedApplication(c, "offered")
	s.addRemoteSecretConsumerApplication(c, offeredAppUUID, "remote-consumer")

	mixed := s.addSecretWithRevisions(c, 3)
	s.saveRemoteSecretConsumer(c, mixed, "remote-consumer/0", 3)
	s.saveRemoteSecretConsumer(c, mixed, "remote-consumer/1", 1)

	current := s.addSecretWithRevisions(c, 2)
	s.saveRemoteSecretConsumer(c, current, "remote-consumer/0", 2)
	s.saveRemoteSecretConsumer(c, current, "remote-consumer/1", 2)

	unconsumed := s.addSecretWithRevisions(c, 2)

	tests := []struct {
		summary  string
		revUUIDs []string
		expected []string
	}{{
		summary:  "revision consumed by every unit",
		revUUIDs: []string{getRevUUID(c, s.DB(), mixed, 1)},
	}, {
		summary:  "revision newer than one unit",
		revUUIDs: []string{getRevUUID(c, s.DB(), mixed, 2)},
		expected: []string{mixed.String()},
	}, {
		summary:  "latest revision",
		revUUIDs: []string{getRevUUID(c, s.DB(), mixed, 3)},
		expected: []string{mixed.String()},
	}, {
		summary: "every revision of each secret",
		revUUIDs: []string{
			getRevUUID(c, s.DB(), mixed, 1),
			getRevUUID(c, s.DB(), mixed, 2),
			getRevUUID(c, s.DB(), mixed, 3),
			getRevUUID(c, s.DB(), current, 1),
			getRevUUID(c, s.DB(), current, 2),
			getRevUUID(c, s.DB(), unconsumed, 2),
		},
		expected: []string{mixed.String()},
	}, {
		summary: "revisions of secrets with no consumer behind",
		revUUIDs: []string{
			getRevUUID(c, s.DB(), current, 2),
			getRevUUID(c, s.DB(), unconsumed, 2),
		},
	}}
	for _, test := range tests {
		c.Logf("test: %s", test.summary)
		uris, err := s.state.GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(
			c.Context(), offeredAppUUID, test.revUUIDs...)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(uris, tc.SameContents, test.expected)
	}
}

// TestRemoteConsumedSecretsChangesApplicationNamePrefix checks that the
// consuming units are matched to their own application, and not to an
// application whose name is a prefix of theirs.
func (s *modelSecretsSuite) TestRemoteConsumedSecretsChangesApplicationNamePrefix(c *tc.C) {
	fooOfferedAppUUID := s.addOfferedApplication(c, "offered-foo")
	s.addRemoteSecretConsumerApplication(c, fooOfferedAppUUID, "remote-foo")
	fooBarOfferedAppUUID := s.addOfferedApplication(c, "offered-foo-bar")
	s.addRemoteSecretConsumerApplication(c, fooBarOfferedAppUUID, "remote-foo-bar")

	fooURI := s.addSecretWithRevisions(c, 2)
	s.saveRemoteSecretConsumer(c, fooURI, "remote-foo/0", 1)
	s.saveRemoteSecretConsumer(c, fooURI, "remote-foo-bar/0", 2)

	fooBarURI := s.addSecretWithRevisions(c, 2)
	s.saveRemoteSecretConsumer(c, fooBarURI, "remote-foo/0", 2)
	s.saveRemoteSecretConsumer(c, fooBarURI, "remote-foo-bar/0", 1)

	s.assertRemoteConsumedSecretsChanges(c, fooOfferedAppUUID,
		[]string{getRevUUID(c, s.DB(), fooURI, 2)},
		[]string{fooURI.String()},
	)
	s.assertRemoteConsumedSecretsChanges(c, fooBarOfferedAppUUID,
		[]string{getRevUUID(c, s.DB(), fooBarURI, 2)},
		[]string{fooBarURI.String()},
	)
}

func (s *modelSecretsSuite) TestSaveSecretRemoteConsumer(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"}, nil)

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
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"}, nil)
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
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"}, nil)

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

func (s *modelSecretsSuite) setupRemoteAppAndRelation(c *tc.C, db database.TxnRunner) (string, string, string, string) {
	localAppUUID, localEpUUID, charmUUID := s.createApplicationForRole(c, db, "mediawiki", 1)
	remoteAppUUID, remoteEpUUID, _ := s.createApplicationForRole(c, db, "mysql", 0)
	relUUID := tc.Must(c, corerelation.NewUUID).String()

	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		remoteOffererUUUID := tc.Must(c, internaluuid.NewUUID).String()
		offerUUID := tc.Must(c, internaluuid.NewUUID).String()
		offerModelUUID := tc.Must(c, internaluuid.NewUUID).String()
		_, err := tx.ExecContext(ctx, `
INSERT INTO application_remote_offerer (uuid, life_id, application_uuid, offer_uuid, offer_url, offerer_model_uuid, macaroon)
VALUES (?, 0, ?, ?, 'offerurl', ?, 'macaroon')`, remoteOffererUUUID, remoteAppUUID, offerUUID, offerModelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, 1, 0)`, relUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, tc.Must(c, internaluuid.NewUUID).String(), relUUID, remoteEpUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, tc.Must(c, internaluuid.NewUUID).String(), relUUID, localEpUUID); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return localAppUUID, remoteAppUUID, relUUID, charmUUID
}

func (s *modelSecretsSuite) createApplicationForRole(c *tc.C, db database.TxnRunner, appName string, roleID int) (string, string, string) {
	appUUID := internaluuid.MustNewUUID().String()
	appEndpointUUID := internaluuid.MustNewUUID().String()
	charmUUID := internaluuid.MustNewUUID().String()

	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, reference_name, architecture_id, revision)
VALUES (?, ?, 0, 1)`, charmUUID, charmUUID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate, description)
VALUES (?, ?, false, 'test app')`, charmUUID, appName); err != nil {
			return err
		}
		charmRelationUUID := internaluuid.MustNewUUID().String()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, capacity, scope_id)
VALUES (?, ?, 'db', ?, 'db', 0, 0)`, charmRelationUUID, charmUUID, roleID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, 0, ?, ?)`, appUUID, appName, charmUUID, network.AlphaSpaceId); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid)
VALUES (?, ?, ?, ?)`, appEndpointUUID, appUUID, charmRelationUUID, network.AlphaSpaceId); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return appUUID, appEndpointUUID, charmUUID
}

func (s *modelSecretsSuite) createUnit(c *tc.C, db database.TxnRunner, unitName, appUUID, charmUUID string) string {
	unitUUID := tc.Must(c, internaluuid.NewUUID).String()
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		netNodeUUID := tc.Must(c, internaluuid.NewUUID).String()
		_, err := tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
		if err != nil {
			return errors.Capture(err)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, life_id, name, net_node_uuid, application_uuid, charm_uuid)
VALUES (?, 0, ?, ?, ?, ?)
`, unitUUID, unitName, netNodeUUID, appUUID, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID
}

func (s *modelSecretsSuite) TestSaveRemoteSecretConsumer(c *tc.C) {
	appUUID, remoteAppUUID, rUUID, charmUUID := s.setupRemoteAppAndRelation(c, s.TxnRunner())
	unitUUID := s.createUnit(c, s.TxnRunner(), "mediawiki/0", appUUID, charmUUID)
	modelUUID := tc.Must(c, internaluuid.NewUUID).String()
	uri := coresecrets.NewURI().WithSource(modelUUID)
	ctx := c.Context()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"}, nil)

	assertSaveRemoteSecretConsumer := func(consumer *coresecrets.SecretConsumerMetadata) {
		err := s.state.SaveRemoteSecretConsumer(ctx, uri, unitUUID, *consumer, appUUID, rUUID)
		c.Assert(err, tc.ErrorIsNil)

		var (
			secretID        string
			currentRevision int
			label           string
		)
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			err := tx.QueryRowContext(ctx, `
SELECT secret_id FROM secret_reference WHERE owner_application_uuid = ?
`, remoteAppUUID).Scan(&secretID)
			if err != nil {
				return err
			}
			err = tx.QueryRowContext(ctx, `
SELECT label, current_revision FROM secret_unit_consumer WHERE secret_id = ?
`, secretID).Scan(&label, &currentRevision)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(secretID, tc.Equals, uri.ID)
		c.Assert(label, tc.Equals, consumer.Label)
		c.Assert(currentRevision, tc.Equals, consumer.CurrentRevision)
	}

	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my label",
		CurrentRevision: 666,
	}
	assertSaveRemoteSecretConsumer(consumer)

	// Second time updates.
	consumer = &coresecrets.SecretConsumerMetadata{
		Label:           "my label2",
		CurrentRevision: 667,
	}
	assertSaveRemoteSecretConsumer(consumer)

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

	appUUID := s.setupRemoteApp(c, "mediawiki")

	// Create secret with 3 revisions (1, 2, 3).
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)
	s.addRevision(c, uri, map[string]string{"foo": "bar2"})
	s.addRevision(c, uri, map[string]string{"foo": "bar3"})

	// Initially revisions are not obsolete.
	for i := 1; i <= 3; i++ {
		obsolete, _ := s.getObsolete(c, uri, i)
		c.Check(obsolete, tc.IsFalse)
	}

	// Update to latest revision 3.
	err := s.state.UpdateRemoteSecretRevision(c.Context(), uri, 3, appUUID)
	c.Assert(err, tc.ErrorIsNil)
	got := getLatest()
	c.Assert(got, tc.Equals, 3)

	// Revisions 1 and 2 are now obsolete because they are not the latest and have no consumers.
	obsolete, _ := s.getObsolete(c, uri, 1)
	c.Check(obsolete, tc.IsTrue)
	obsolete, _ = s.getObsolete(c, uri, 2)
	c.Check(obsolete, tc.IsTrue)

	// Revision 3 is NOT obsolete because it's the latest in secret_revision table.
	obsolete, _ = s.getObsolete(c, uri, 3)
	c.Check(obsolete, tc.IsFalse)

	// Add revision 4.
	s.addRevision(c, uri, map[string]string{"foo": "bar4"})

	// Add a consumer for revision 3.
	consumer := coresecrets.SecretConsumerMetadata{
		CurrentRevision: 3,
	}
	err = s.state.SaveSecretRemoteConsumer(c.Context(), uri, "remote-app/0", consumer)
	c.Assert(err, tc.ErrorIsNil)

	// Update to latest revision 4.
	err = s.state.UpdateRemoteSecretRevision(c.Context(), uri, 4, appUUID)
	c.Assert(err, tc.ErrorIsNil)
	got = getLatest()
	c.Assert(got, tc.Equals, 4)

	// Revision 3 should NOT be obsolete because it has a consumer.
	obsolete, _ = s.getObsolete(c, uri, 3)
	c.Check(obsolete, tc.IsFalse)

	// Revision 4 should NOT be obsolete because it's the latest in secret_revision table.
	obsolete, _ = s.getObsolete(c, uri, 4)
	c.Check(obsolete, tc.IsFalse)
}

func (s *modelSecretsSuite) setupSecretAccess(c *tc.C, uri *coresecrets.URI, unitName coreunit.Name) {
	charmUUID := s.addCharm(c)
	s.addCharmMetadataWithDescription(c, charmUUID, "testing application")
	rel := charm.Relation{
		Name:      "db-admin",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, rel)

	appUUID := s.addApplication(c, charmUUID, unitName.Application())
	err := s.state.EnsureUnitsExist(c.Context(), appUUID.String(), []string{unitName.String()})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		modelUUID := tc.Must0(c, coremodel.NewUUID)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid,  name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		if err != nil {
			return err
		}

		var unitUUID string
		err = tx.QueryRowContext(ctx, `SELECT uuid FROM unit WHERE name = ?`, unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO secret_permission(secret_id, role_id, subject_uuid, subject_type_id, scope_uuid, scope_type_id)
			VALUES(?, ?, ?, ?, ?, ?)
		`, uri.ID, domainsecret.RoleView, unitUUID, domainsecret.SubjectUnit, relationUUID, domainsecret.ScopeRelation)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretsSuite) TestGetSecretAccess(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"}, nil)
	s.setupSecretAccess(c, uri, tc.Must1(c, coreunit.NewName, "mediawiki/0"))

	access, err := s.state.GetSecretAccess(c.Context(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mediawiki/0",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.DeepEquals, "view")
}

func (s *modelSecretsSuite) TestGetSecretAccessNone(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar", "hello": "world"}, nil)
	s.setupSecretAccess(c, uri, tc.Must1(c, coreunit.NewName, "mediawiki/0"))

	access, err := s.state.GetSecretAccess(c.Context(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.DeepEquals, "")
}

func (s *modelSecretsSuite) TestGetSecretAccessNotFound(c *tc.C) {
	uri := coresecrets.NewURI()

	_, err := s.state.GetSecretAccess(c.Context(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "unit/0",
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

func (s *modelSecretsSuite) TestGetSecretValueWithContent(c *tc.C) {
	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar", "hello": "world"}
	s.createSecret(c, uri, data, nil)

	got, ref, err := s.state.GetSecretValue(c.Context(), uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(got, tc.DeepEquals, coresecrets.SecretData(data))
}

func (s *modelSecretsSuite) TestGetSecretValueRef(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, nil, &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	})

	val, ref, err := s.state.GetSecretValue(c.Context(), uri, 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(val, tc.IsNil)
	c.Assert(ref, tc.DeepEquals, &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	})
}

func (s *modelSecretsSuite) TestGetSecretValueRevisionNotFound(c *tc.C) {
	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar", "hello": "world"}
	s.createSecret(c, uri, data, nil)

	_, _, err := s.state.GetSecretValue(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretRevisionNotFound)
}

// addOfferConnection adds an offer connection of a consuming application to
// the database, with its own offer, synthetic application and relation, and
// returns their UUIDs. The synthetic application is named as given.
func (s *modelSecretsSuite) addOfferConnection(
	c *tc.C, consumeAppUUID, offererAppUUID, synthAppName string,
) (offerUUID, relUUID, synthAppUUID string) {
	offerUUID = tc.Must(c, internaluuid.NewUUID).String()
	s.createOffer(c, offerUUID)

	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createCharm(c, charmUUID)
	synthAppUUID = s.addApplication(c, corecharm.ID(charmUUID), synthAppName).String()

	relUUID = s.addRelation(c).String()

	s.query(c, `
INSERT INTO offer_connection (uuid, offer_uuid, remote_relation_uuid, username)
VALUES (?, ?, ?, 'bob')`, synthAppUUID, offerUUID, relUUID)

	s.query(c, `
INSERT INTO application_remote_consumer (
    offer_connection_uuid,
    offerer_application_uuid,
    consumer_application_uuid,
    consumer_model_uuid,
    life_id)
VALUES (?, ?, ?, ?, 0)`,
		synthAppUUID, offererAppUUID, consumeAppUUID, tc.Must0(c, coremodel.NewUUID))

	return offerUUID, relUUID, synthAppUUID
}

// grantSecretToConsumer records a grant of the secret to the synthetic
// application of an offer connection of a consuming application, scoped by
// the relation of the connection, as a remote application consumer is
// granted access through the application over a relation.
func (s *modelSecretsSuite) grantSecretToConsumer(c *tc.C, uri *coresecrets.URI, synthAppUUID, relUUID string) {
	s.query(c, `
INSERT INTO secret_permission (secret_id, role_id, subject_uuid, subject_type_id, scope_uuid, scope_type_id)
VALUES (?, ?, ?, ?, ?, ?)`,
		uri.ID, domainsecret.RoleView, synthAppUUID, domainsecret.SubjectApplication, relUUID, domainsecret.ScopeRelation)
}

// TestGetRemoteConsumerSecretGrants checks that a grant of a secret to a
// remote consumer resolves the offer connection the secret was granted
// over: the synthetic application representing the consuming application
// for that connection, the relation scoping the grant, and the offer the
// connection was made for.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrants(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	offerUUID, relUUID, _, synthAppUUID, consumeAppUUID := s.setupRemoteApplicationConsumer(c)
	s.grantSecretToConsumer(c, uri, synthAppUUID, relUUID)

	grants, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(grants, tc.DeepEquals, []crossmodelrelation.RemoteConsumerSecretGrant{{
		// The helper names the synthetic application by its UUID.
		ApplicationName: synthAppUUID,
		RelationUUID:    tc.Must1(c, corerelation.ParseUUID, relUUID),
		OfferUUID:       tc.Must1(c, coreoffer.ParseUUID, offerUUID),
	}})
}

// TestGetRemoteConsumerSecretGrantsMultipleOfferConnections checks that the
// consuming application of a secret grant is resolved per offer connection
// when a consuming application holds several connections, as migrated from
// a 3.6 consumer proxy relating to several offered applications: the
// consuming application UUID is shared by every connection, so the grant
// resolves the connection it was made for, not an arbitrary one.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsMultipleOfferConnections(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	_, _, offererAppUUID, synthAppUUID1, consumeAppUUID := s.setupRemoteApplicationConsumer(c)
	// Add a second offer connection of the same consuming application.
	offerUUID2, relUUID2, synthAppUUID2 := s.addOfferConnection(c, consumeAppUUID, offererAppUUID, "remote-second")

	// The secret is granted over the second connection only: the grant
	// must resolve that connection, not the first one.
	s.grantSecretToConsumer(c, uri, synthAppUUID2, relUUID2)

	grants, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(grants, tc.DeepEquals, []crossmodelrelation.RemoteConsumerSecretGrant{{
		ApplicationName: "remote-second",
		RelationUUID:    tc.Must1(c, corerelation.ParseUUID, relUUID2),
		OfferUUID:       tc.Must1(c, coreoffer.ParseUUID, offerUUID2),
	}})
	c.Check(synthAppUUID1, tc.Not(tc.Equals), synthAppUUID2)
}

// TestGetRemoteConsumerSecretGrantsOrdered checks that the grants of a
// secret granted over several offer connections of a consuming application
// are returned in a deterministic order, ordered by the name of the
// synthetic application of each connection.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsOrdered(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	consumeAppUUID := tc.Must0(c, coreapplication.NewUUID).String()
	offererAppUUID := s.setupRemoteApp(c, "mysql")
	offerUUID1, relUUID1, synthApp1 := s.addOfferConnection(c, consumeAppUUID, offererAppUUID, "remote-zzz")
	offerUUID2, relUUID2, synthApp2 := s.addOfferConnection(c, consumeAppUUID, offererAppUUID, "remote-aaa")
	s.grantSecretToConsumer(c, uri, synthApp1, relUUID1)
	s.grantSecretToConsumer(c, uri, synthApp2, relUUID2)

	grants, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(grants, tc.DeepEquals, []crossmodelrelation.RemoteConsumerSecretGrant{
		{
			ApplicationName: "remote-aaa",
			RelationUUID:    tc.Must1(c, corerelation.ParseUUID, relUUID2),
			OfferUUID:       tc.Must1(c, coreoffer.ParseUUID, offerUUID2),
		}, {
			ApplicationName: "remote-zzz",
			RelationUUID:    tc.Must1(c, corerelation.ParseUUID, relUUID1),
			OfferUUID:       tc.Must1(c, coreoffer.ParseUUID, offerUUID1),
		},
	})
}

// TestGetRemoteConsumerSecretGrantsAccessScopeNotFound checks that an error
// satisfying [secreterrors.SecretAccessScopeNotFound] is returned when the
// consuming application of the given UUID holds no grant of the secret, even
// when it holds a grant of another secret.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsAccessScopeNotFound(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	otherURI := coresecrets.NewURI()
	s.createSecret(c, otherURI, map[string]string{"foo": "bar"}, nil)

	_, relUUID, _, synthAppUUID, consumeAppUUID := s.setupRemoteApplicationConsumer(c)
	// The consumer is granted another secret, which must not resolve as
	// a grant of the first one.
	s.grantSecretToConsumer(c, otherURI, synthAppUUID, relUUID)

	_, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretAccessScopeNotFound)
}

// TestGetRemoteConsumerSecretGrantsSecretNotFound checks that an error
// satisfying [secreterrors.SecretNotFound] is returned when the secret does
// not exist.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsSecretNotFound(c *tc.C) {
	uri := coresecrets.NewURI()

	_, _, _, _, consumeAppUUID := s.setupRemoteApplicationConsumer(c)

	_, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
}

// TestGetRemoteConsumerSecretGrantsConsumerIsolation checks that the
// grants of a secret are resolved per consuming application: when several
// consuming applications are granted the same secret, each lookup
// returns only the grants over the connections of its own application.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsConsumerIsolation(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	offererAppUUID := s.setupRemoteApp(c, "mysql")

	consumeAppUUID1 := tc.Must0(c, coreapplication.NewUUID).String()
	offerUUID1, relUUID1, synthApp1 := s.addOfferConnection(c, consumeAppUUID1, offererAppUUID, "remote-one")
	s.grantSecretToConsumer(c, uri, synthApp1, relUUID1)

	consumeAppUUID2 := tc.Must0(c, coreapplication.NewUUID).String()
	offerUUID2, relUUID2, synthApp2 := s.addOfferConnection(c, consumeAppUUID2, offererAppUUID, "remote-two")
	s.grantSecretToConsumer(c, uri, synthApp2, relUUID2)

	grants, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID1)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(grants, tc.DeepEquals, []crossmodelrelation.RemoteConsumerSecretGrant{{
		ApplicationName: "remote-one",
		RelationUUID:    tc.Must1(c, corerelation.ParseUUID, relUUID1),
		OfferUUID:       tc.Must1(c, coreoffer.ParseUUID, offerUUID1),
	}})

	grants, err = s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(grants, tc.DeepEquals, []crossmodelrelation.RemoteConsumerSecretGrant{{
		ApplicationName: "remote-two",
		RelationUUID:    tc.Must1(c, corerelation.ParseUUID, relUUID2),
		OfferUUID:       tc.Must1(c, coreoffer.ParseUUID, offerUUID2),
	}})
}

// TestGetRemoteConsumerSecretGrantsInvalidRelationUUID checks that an
// error is returned when the relation UUID scoping a grant is not a
// valid UUID.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsInvalidRelationUUID(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	_, _, _, synthAppUUID, consumeAppUUID := s.setupRemoteApplicationConsumer(c)
	// The grant is scoped by a relation UUID which is not a valid UUID.
	s.grantSecretToConsumer(c, uri, synthAppUUID, "not a uuid")

	_, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetRemoteConsumerSecretGrantsInvalidOfferUUID checks that an
// error is returned when the offer UUID of the connection a grant was
// made over is not a valid UUID.
func (s *modelSecretsSuite) TestGetRemoteConsumerSecretGrantsInvalidOfferUUID(c *tc.C) {
	uri := coresecrets.NewURI()
	s.createSecret(c, uri, map[string]string{"foo": "bar"}, nil)

	consumeAppUUID := tc.Must0(c, coreapplication.NewUUID).String()
	offererAppUUID := s.setupRemoteApp(c, "mysql")

	// The offer of the connection is identified by a value which is not
	// a valid UUID.
	s.createOffer(c, "not a uuid")
	charmUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.createCharm(c, charmUUID)
	synthAppUUID := s.addApplication(c, corecharm.ID(charmUUID), "remote-invalid").String()
	relUUID := s.addRelation(c).String()

	s.query(c, `
INSERT INTO offer_connection (uuid, offer_uuid, remote_relation_uuid, username)
VALUES (?, ?, ?, 'bob')`, synthAppUUID, "not a uuid", relUUID)

	s.query(c, `
INSERT INTO application_remote_consumer (
    offer_connection_uuid,
    offerer_application_uuid,
    consumer_application_uuid,
    consumer_model_uuid,
    life_id)
VALUES (?, ?, ?, ?, 0)`,
		synthAppUUID, offererAppUUID, consumeAppUUID, tc.Must0(c, coremodel.NewUUID))

	s.grantSecretToConsumer(c, uri, synthAppUUID, relUUID)

	_, err := s.state.GetRemoteConsumerSecretGrants(c.Context(), uri, consumeAppUUID)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}
