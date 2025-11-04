// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/charm"
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
	s.createSecret(c, uri1, map[string]string{"foo": "bar", "hello": "world"}, nil)
	uri1.SourceUUID = s.ModelUUID()

	uri2 := coresecrets.NewURI()
	s.createSecret(c, uri2, map[string]string{"foo": "bar", "hello": "world"}, nil)
	uri2.SourceUUID = s.ModelUUID()

	appUUID := s.setupRemoteApp(c, "mediawiki")

	// The consumed revision 1.
	saveRemoteConsumer(uri1, 1, "mediawiki/0")
	// The consumed revision 1.
	saveRemoteConsumer(uri2, 1, "mediawiki/0")

	// create revision 2.
	s.addRevision(c, uri1, map[string]string{"foo": "bar2"})

	err := s.state.UpdateRemoteSecretRevision(ctx, uri1, 2, appUUID)
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
	err := s.state.UpdateRemoteSecretRevision(c.Context(), uri, 666, appUUID)
	c.Assert(err, tc.ErrorIsNil)
	got := getLatest()
	c.Assert(got, tc.Equals, 666)
	err = s.state.UpdateRemoteSecretRevision(c.Context(), uri, 667, appUUID)
	c.Assert(err, tc.ErrorIsNil)
	got = getLatest()
	c.Assert(got, tc.Equals, 667)
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
		modelUUID := modeltesting.GenModelUUID(c)
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
