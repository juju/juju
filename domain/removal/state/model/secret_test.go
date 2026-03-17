// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type secretSuite struct {
	baseSuite
}

func TestSecretSuite(t *testing.T) {
	tc.Run(t, &secretSuite{})
}

func (s *secretSuite) TestDeleteUnitOwnedSecrets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()
	app, unit := s.addAppAndUnit(c)
	sec := s.addSecretWithRevisionsAndContent(c, app)

	_, err := s.DB().ExecContext(
		ctx,
		"INSERT INTO secret_unit_owner (secret_id, unit_uuid) VALUES (?, ?)",
		sec, unit,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteUnitOwnedSecretContent(ctx, unit)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteUnitOwnedSecrets(ctx, unit)
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(ctx, "SELECT count(*) FROM secret")

	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *secretSuite) TestDeleteApplicationOwnedSecrets(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, _ := s.addAppAndUnit(c)
	sec := s.addSecretWithRevisionsAndContent(c, app)

	_, err := s.DB().ExecContext(
		ctx, "INSERT INTO secret_application_owner (secret_id, application_uuid) VALUES (?, ?)", sec, app)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteApplicationOwnedSecretContent(ctx, app)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteApplicationOwnedSecrets(ctx, app)
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(ctx, "SELECT count(*) FROM secret")

	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *secretSuite) TestGetApplicationOwnedSecretRevisionRefs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, _ := s.addAppAndUnit(c)
	sec := s.addSecretWithRevisionsAndContent(c, app)

	_, err := s.DB().ExecContext(
		ctx, "INSERT INTO secret_application_owner (secret_id, application_uuid) VALUES (?, ?)", sec, app)
	c.Assert(err, tc.ErrorIsNil)

	ids, err := st.GetApplicationOwnedSecretRevisionRefs(ctx, app)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.SameContents, []string{"0", "1", "2"})
}

func (s *secretSuite) TestGetUnitOwnedSecretRevisionRefs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, unit := s.addAppAndUnit(c)
	sec := s.addSecretWithRevisionsAndContent(c, app)

	_, err := s.DB().ExecContext(
		ctx, "INSERT INTO secret_unit_owner (secret_id, unit_uuid) VALUES (?, ?)", sec, unit)
	c.Assert(err, tc.ErrorIsNil)

	ids, err := st.GetUnitOwnedSecretRevisionRefs(ctx, unit)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.SameContents, []string{"0", "1", "2"})
}

func (s *secretSuite) TestDeleteSomeRevisions(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, _ := s.addAppAndUnit(c)
	sec := s.addSecretWithRevisionsAndContent(c, app)
	uri := &coresecrets.URI{ID: sec}

	deleted, err := st.DeleteUserSecretRevisions(ctx, uri, []int{1})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deleted, tc.DeepEquals, []string{"revision_id_1"})

	s.checkRevisionExists(c, uri, 0)
	s.checkRevisionDoesNotExist(c, uri, 1)
	s.checkRevisionExists(c, uri, 2)

	s.checkCount(c, "secret_revision", 2)
	s.checkCount(c, "secret_content", 2)
	s.checkCount(c, "secret_value_ref", 2)
	s.checkCount(c, "secret_revision_expire", 2)
	s.checkCount(c, "secret_revision_obsolete", 2)
	s.checkCount(c, "secret_reference", 1)
	s.checkCount(c, "secret", 1)
}

func (s *secretSuite) TestDeleteAllRevisionsFromNil(c *tc.C) {
	s.assertDeleteAllRevisions(c, nil)
}

func (s *secretSuite) TestDeleteAllRevisions(c *tc.C) {
	s.assertDeleteAllRevisions(c, []int{0, 1, 2})
}

func (s *secretSuite) assertDeleteAllRevisions(c *tc.C, revs []int) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, unit := s.addAppAndUnit(c)
	sec := s.addSecretWithRevisionsAndContent(c, app)
	uri := &coresecrets.URI{ID: sec}

	q := "INSERT INTO secret_application_owner (secret_id, application_uuid) VALUES (?, ?)"
	_, err := s.DB().ExecContext(ctx, q, sec, app)
	c.Assert(err, tc.ErrorIsNil)

	q = "INSERT INTO secret_unit_owner (secret_id, unit_uuid) VALUES (?, ?)"
	_, err = s.DB().ExecContext(ctx, q, sec, unit)
	c.Assert(err, tc.ErrorIsNil)

	deleted, err := st.DeleteUserSecretRevisions(ctx, uri, revs)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deleted, tc.SameContents, []string{"revision_id_0", "revision_id_1", "revision_id_2"})

	s.checkCount(c, "secret_revision", 0)
	s.checkCount(c, "secret_content", 0)
	s.checkCount(c, "secret_value_ref", 0)
	s.checkCount(c, "secret_revision_expire", 0)
	s.checkCount(c, "secret_revision_obsolete", 0)
	s.checkCount(c, "secret_reference", 0)
	s.checkCount(c, "secret_rotation", 0)
	s.checkCount(c, "secret_metadata", 0)
	s.checkCount(c, "secret_application_owner", 0)
	s.checkCount(c, "secret_unit_owner", 0)
	s.checkCount(c, "secret", 0)
}

func (s *secretSuite) checkCount(c *tc.C, table string, expected int) {
	row := s.DB().QueryRowContext(c.Context(), "SELECT count(*) FROM "+table)
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected)
}

func (s *secretSuite) TestDeleteObsoleteUserSecretRevisions(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()
	app, _ := s.addAppAndUnit(c)

	uriNoAuto, _ := s.addSecretWithRevisions(c, app, false, true, map[int]bool{1: true, 2: false})
	uriAuto, revUUIDsAuto := s.addSecretWithRevisions(c, app, true, true, map[int]bool{1: true, 2: false})
	uriAutoNoObsolete, _ := s.addSecretWithRevisions(c, app, true, true, map[int]bool{1: false})
	uriCharmAuto, _ := s.addSecretWithRevisions(c, app, true, false, map[int]bool{1: true, 2: false})

	deletedRevisionUUIDs, err := st.DeleteObsoleteUserSecretRevisions(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deletedRevisionUUIDs, tc.DeepEquals, []string{revUUIDsAuto[1]})

	s.checkRevisionExists(c, uriNoAuto, 1)
	s.checkRevisionExists(c, uriNoAuto, 2)
	s.checkRevisionDoesNotExist(c, uriAuto, 1)
	s.checkRevisionExists(c, uriAuto, 2)
	s.checkRevisionExists(c, uriAutoNoObsolete, 1)
	s.checkRevisionExists(c, uriCharmAuto, 1)
	s.checkRevisionExists(c, uriCharmAuto, 2)
}

func (s *secretSuite) TestGetUserSecretRevisionRefs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, _ := s.addAppAndUnit(c)
	_ = s.addSecretWithRevisionsAndContent(c, app)

	refs, err := st.GetUserSecretRevisionRefs(ctx, []string{"revision_id_2", "revision_id_0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(refs, tc.SameContents, []string{"0", "2"})
}

func (s *secretSuite) TestDeleteUserSecretRevisionRef(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	app, _ := s.addAppAndUnit(c)
	_ = s.addSecretWithRevisionsAndContent(c, app)

	err := st.DeleteUserSecretRevisionRef(ctx, "1")
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRowContext(
		ctx,
		"SELECT count(*) FROM secret_deleted_value_ref WHERE revision_uuid = ?",
		"revision_id_1",
	)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *secretSuite) addSecretWithRevisionsAndContent(c *tc.C, appUUID string) string {
	ctx := c.Context()

	sec := "secret_id"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO secret VALUES (?)", sec)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO secret_metadata (secret_id, version, rotate_policy_id) VALUES (?, ?, ?)", sec, 1, 0)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, `
INSERT INTO secret_reference (secret_id, latest_revision, owner_application_uuid, updated_at) 
VALUES (?, ?, ?, ?)`, sec, 0, appUUID, time.Now().UTC())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO secret_rotation (secret_id, next_rotation_time) VALUES (?, DATETIME('now'))", sec)
	c.Assert(err, tc.ErrorIsNil)

	for i := range 3 {
		rev := "revision_id_" + strconv.Itoa(i)

		_, err := s.DB().ExecContext(
			ctx, "INSERT INTO secret_revision (uuid, secret_id, revision) VALUES (?, ?, ?)", rev, sec, i)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.DB().ExecContext(
			ctx,
			"INSERT INTO secret_value_ref (revision_uuid, backend_uuid, revision_id) VALUES (?, ?, ?)",
			rev, "backend-uuid", i,
		)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.DB().ExecContext(
			ctx,
			"INSERT INTO secret_deleted_value_ref (revision_uuid, backend_uuid, revision_id) VALUES (?, ?, ?)",
			rev, "backend-uuid", i,
		)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.DB().ExecContext(
			ctx, "INSERT INTO secret_revision_obsolete (revision_uuid) VALUES (?)", rev)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.DB().ExecContext(
			ctx, "INSERT INTO secret_revision_expire (revision_uuid, expire_time) VALUES (?, DATETIME('now'))", rev)
		c.Assert(err, tc.ErrorIsNil)

		_, err = s.DB().ExecContext(
			ctx,
			"INSERT INTO secret_content (revision_uuid, name, content) VALUES (?, ?, ?)",
			rev, "name", "random-secret-content",
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	return sec
}

func (s *secretSuite) addSecretWithRevisions(
	c *tc.C,
	appUUID string,
	autoPrune bool,
	modelOwned bool,
	obsoleteByRevision map[int]bool,
) (*coresecrets.URI, map[int]string) {
	ctx := c.Context()

	uri := coresecrets.NewURI()
	_, err := s.DB().ExecContext(ctx, "INSERT INTO secret VALUES (?)", uri.ID)
	c.Assert(err, tc.ErrorIsNil)

	q := "INSERT INTO secret_metadata (secret_id, version, rotate_policy_id, auto_prune) VALUES (?, ?, ?, ?)"
	_, err = s.DB().ExecContext(ctx, q, uri.ID, 1, 0, autoPrune)
	c.Assert(err, tc.ErrorIsNil)

	if modelOwned {
		q := "INSERT INTO secret_model_owner (secret_id) VALUES (?)"
		_, err = s.DB().ExecContext(ctx, q, uri.ID)
	} else {
		q := "INSERT INTO secret_application_owner (secret_id, application_uuid) VALUES (?, ?)"
		_, err = s.DB().ExecContext(ctx, q, uri.ID, appUUID)
	}
	c.Assert(err, tc.ErrorIsNil)

	revUUIDs := make(map[int]string, len(obsoleteByRevision))
	for revision, obsolete := range obsoleteByRevision {
		revUUID := uri.ID + "_revision_" + strconv.Itoa(revision)
		revUUIDs[revision] = revUUID

		q := "INSERT INTO secret_revision (uuid, secret_id, revision) VALUES (?, ?, ?)"
		_, err := s.DB().ExecContext(ctx, q, revUUID, uri.ID, revision)
		c.Assert(err, tc.ErrorIsNil)

		q = "INSERT INTO secret_content (revision_uuid, name, content) VALUES (?, ?, ?)"
		_, err = s.DB().ExecContext(ctx, q, revUUID, "name", "content-"+strconv.Itoa(revision))
		c.Assert(err, tc.ErrorIsNil)

		q = "INSERT INTO secret_value_ref (revision_uuid, backend_uuid, revision_id) VALUES (?, ?, ?)"
		_, err = s.DB().ExecContext(ctx, q, revUUID, "backend-uuid", revUUID)
		c.Assert(err, tc.ErrorIsNil)

		q = "INSERT INTO secret_revision_obsolete (revision_uuid, obsolete) VALUES (?, ?)"
		_, err = s.DB().ExecContext(ctx, q, revUUID, obsolete)
		c.Assert(err, tc.ErrorIsNil)
	}

	return uri, revUUIDs
}

func (s *secretSuite) checkRevisionExists(c *tc.C, uri *coresecrets.URI, revision int) {
	c.Check(s.selectRevisionCount(c, uri, revision), tc.Equals, 1)
}

func (s *secretSuite) checkRevisionDoesNotExist(c *tc.C, uri *coresecrets.URI, revision int) {
	c.Check(s.selectRevisionCount(c, uri, revision), tc.Equals, 0)
}

func (s *secretSuite) selectRevisionCount(c *tc.C, uri *coresecrets.URI, revision int) int {
	q := "SELECT count(*) FROM secret_revision WHERE secret_id = ? AND revision = ?"
	row := s.DB().QueryRowContext(c.Context(), q, uri.ID, revision)

	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)

	return count
}

func (s *secretSuite) addAppAndUnit(c *tc.C) (string, string) {
	ctx := c.Context()

	charmUUID := "charm-uuid"
	q := "INSERT INTO charm (uuid, reference_name, source_id, architecture_id) VALUES (?, ?, ?, ?)"
	_, err := s.DB().ExecContext(ctx, q, charmUUID, charmUUID, 1, 0)
	c.Assert(err, tc.ErrorIsNil)

	appUUID := "app-uuid"
	q = "INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)"
	_, err = s.DB().ExecContext(ctx, q, appUUID, appUUID, 0, charmUUID, network.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)

	nodeUUID := "net-node-uuid"
	_, err = s.DB().Exec("INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := "unit-uuid"
	q = "INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)"
	_, err = s.DB().Exec(q, unitUUID, unitUUID, 0, appUUID, charmUUID, nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, unitUUID
}
