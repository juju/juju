// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accessstate "github.com/juju/juju/domain/access/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// seedRedirect inserts a migration redirect snapshot row for the given model,
// optionally completed, plus captured access rows for the given
// user-uuid -> user-name pairs. It returns the target controller UUID.
func (m *stateSuite) seedRedirect(
	c *tc.C, modelUUID coremodel.UUID, completed bool, users map[string]string,
) string {
	targetControllerUUID := uuid.MustNewUUID().String()
	err := m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var completedAt sql.NullString
		if completed {
			completedAt = sql.NullString{String: "2026-07-08 00:00:00", Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_redirect (model_uuid, source_migration_uuid,
    target_controller_uuid, target_controller_alias, target_addresses,
    target_ca_cert, created_at, completed_at)
VALUES (?, ?, ?, 'target-alias', '10.0.0.1:17070,[2001:db8::1]:17070',
    'ca-cert-data', DATETIME('now', 'utc'), ?)`,
			modelUUID, uuid.MustNewUUID().String(), targetControllerUUID, completedAt); err != nil {
			return err
		}
		for userUUID, name := range users {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_redirect_user (model_uuid, user_uuid, user_name, access)
VALUES (?, ?, ?, 'admin')`,
				modelUUID, userUUID, name); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return targetControllerUUID
}

// addRedirectUser inserts an enabled user and returns its UUID.
func (m *stateSuite) addRedirectUser(c *tc.C, name string) user.UUID {
	accessState := accessstate.NewState(m.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	userUUID := usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		c.Context(), userUUID, usertesting.GenNewName(c, name), name, false, m.userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	return userUUID
}

// TestGetModelRedirectionNotRedirected asserts a model without any redirect
// snapshot is reported as not redirected.
func (m *stateSuite) TestGetModelRedirectionNotRedirected(c *tc.C) {
	_, err := m.modelState.GetModelRedirection(c.Context(), tc.Must(c, coremodel.NewUUID))
	c.Assert(err, tc.ErrorIs, modelerrors.ModelNotRedirected)
}

// TestGetModelRedirectionStagedNotActive asserts a staged-but-incomplete
// redirect (completed_at IS NULL) is not active.
func (m *stateSuite) TestGetModelRedirectionStagedNotActive(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	m.seedRedirect(c, modelUUID, false, nil)

	_, err := m.modelState.GetModelRedirection(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, modelerrors.ModelNotRedirected)
}

// TestGetModelRedirection asserts a completed redirect round-trips the target
// controller details, including the comma-separated address list.
func (m *stateSuite) TestGetModelRedirection(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	targetControllerUUID := m.seedRedirect(c, modelUUID, true, nil)

	redirection, err := m.modelState.GetModelRedirection(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(redirection.ControllerUUID, tc.Equals, targetControllerUUID)
	c.Check(redirection.ControllerAlias, tc.Equals, "target-alias")
	c.Check(redirection.Addresses, tc.DeepEquals, []string{"10.0.0.1:17070", "[2001:db8::1]:17070"})
	c.Check(redirection.CACert, tc.Equals, "ca-cert-data")
}

// TestGetModelRedirectUsers asserts captured users are returned with their
// access, restricted to users who can still log in: users removed or disabled
// since the snapshot was taken are excluded, as are captured rows whose user
// no longer exists.
func (m *stateSuite) TestGetModelRedirectUsers(c *tc.C) {
	disabledUUID := m.addRedirectUser(c, "disabled-user")
	removedUUID := m.addRedirectUser(c, "removed-user")
	err := m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE user_authentication SET disabled = true WHERE user_uuid = ?",
			disabledUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"UPDATE user SET removed = true WHERE uuid = ?", removedUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	modelUUID := tc.Must(c, coremodel.NewUUID)
	m.seedRedirect(c, modelUUID, true, map[string]string{
		m.userUUID.String():         m.userName.Name(),
		disabledUUID.String():       "disabled-user",
		removedUUID.String():        "removed-user",
		uuid.MustNewUUID().String(): "vanished-user",
	})

	users, err := m.modelState.GetModelRedirectUsers(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.HasLen, 1)
	c.Check(users[0].UserName, tc.Equals, m.userName.Name())
	c.Check(users[0].Access, tc.Equals, "admin")
}

// TestGetModelRedirectUsersEmpty asserts a redirect with no captured users
// yields an empty result rather than an error.
func (m *stateSuite) TestGetModelRedirectUsersEmpty(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	m.seedRedirect(c, modelUUID, true, nil)

	users, err := m.modelState.GetModelRedirectUsers(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(users, tc.HasLen, 0)
}
