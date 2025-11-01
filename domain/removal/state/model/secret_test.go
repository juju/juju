// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"strconv"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
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
	sec := s.addSecretWithRevisionsAndContent(c)
	_, unit := s.addAppAndUnit(c)

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

	sec := s.addSecretWithRevisionsAndContent(c)
	app, _ := s.addAppAndUnit(c)

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

	sec := s.addSecretWithRevisionsAndContent(c)
	app, _ := s.addAppAndUnit(c)

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

	sec := s.addSecretWithRevisionsAndContent(c)
	_, unit := s.addAppAndUnit(c)

	_, err := s.DB().ExecContext(
		ctx, "INSERT INTO secret_unit_owner (secret_id, unit_uuid) VALUES (?, ?)", sec, unit)
	c.Assert(err, tc.ErrorIsNil)

	ids, err := st.GetUnitOwnedSecretRevisionRefs(ctx, unit)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(ids, tc.SameContents, []string{"0", "1", "2"})
}

func (s *secretSuite) addSecretWithRevisionsAndContent(c *tc.C) string {
	ctx := c.Context()

	sec := "secret_id"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO secret VALUES (?)", sec)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO secret_metadata (secret_id, version, rotate_policy_id) VALUES (?, ?, ?)", sec, 1, 0)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO secret_reference (secret_id, latest_revision) VALUES (?, ?)", sec, 0)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO secret_rotation (secret_id, next_rotation_time) VALUES (?, DATETIME('now'))", sec)
	c.Assert(err, tc.ErrorIsNil)

	for i := 0; i < 3; i++ {
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

func (s *secretSuite) addAppAndUnit(c *tc.C) (string, string) {
	ctx := c.Context()

	charmUUID := "charm-uuid"
	_, err := s.DB().ExecContext(
		ctx,
		"INSERT INTO charm (uuid, reference_name, source_id, architecture_id) VALUES (?, ?, ?, ?)",
		charmUUID, charmUUID, 1, 0)
	c.Assert(err, tc.ErrorIsNil)

	appUUID := "app-uuid"
	_, err = s.DB().ExecContext(
		ctx,
		"INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)",
		appUUID, appUUID, 0, charmUUID, network.AlphaSpaceId,
	)
	c.Assert(err, tc.ErrorIsNil)

	nodeUUID := "net-node-uuid"
	_, err = s.DB().Exec("INSERT INTO net_node (uuid) VALUES (?)", nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID := "unit-uuid"
	_, err = s.DB().Exec(
		"INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID, unitUUID, 0, appUUID, charmUUID, nodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, unitUUID
}
