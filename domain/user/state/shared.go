// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	permissionstate "github.com/juju/juju/domain/permission/state"
	usererrors "github.com/juju/juju/domain/user/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// SharedState is the shared state for the user domain. Methods which are shared
// between multiple domains.
// This composes the permission state as it is shared between user and
// permission domains.
type SharedState struct {
	statementBase   internaldatabase.StatementBase
	permissionState *permissionstate.SharedState
}

// NewSharedState creates a new SharedState.
func NewSharedState(base internaldatabase.StatementBase) *SharedState {
	return &SharedState{
		statementBase:   base,
		permissionState: permissionstate.NewSharedState(base),
	}
}

// AddUser adds a new user to the database. If the user already exists an error
// that satisfies usererrors.AlreadyExists will be returned. If the creator does
// not exist an error that satisfies usererrors.UserCreatorUUIDNotFound will be
// returned.
func (s *SharedState) AddUser(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUuid user.UUID,
	access permission.AccessSpec,
) error {
	permissionUUID, err := internaluuid.NewUUID()
	if err != nil {
		return errors.Annotate(err, "generating permission UUID")
	}

	addUserQuery := `
INSERT INTO user (uuid, name, display_name, created_by_uuid, created_at) 
VALUES      ($M.uuid, $M.name, $M.display_name, $M.created_by_uuid, $M.created_at)`

	insertAddUserStmt, err := s.statementBase.Prepare(addUserQuery, sqlair.M{})
	if err != nil {
		return errors.Annotate(err, "preparing add user query")
	}

	err = tx.Query(ctx, insertAddUserStmt, sqlair.M{
		"uuid":            uuid.String(),
		"name":            name,
		"display_name":    displayName,
		"created_by_uuid": creatorUuid.String(),
		"created_at":      time.Now(),
	}).Run()
	if internaldatabase.IsErrConstraintUnique(err) {
		return errors.Annotatef(usererrors.AlreadyExists, "adding user %q", name)
	} else if internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Annotatef(usererrors.CreatorUUIDNotFound, "adding user %q", name)
	} else if err != nil {
		return errors.Annotatef(err, "adding user %q", name)
	}

	err = s.permissionState.AddUserPermission(ctx, tx, permissionstate.AddUserPermissionArgs{
		PermissionUUID: permissionUUID.String(),
		UserUUID:       uuid.String(),
		Access:         access.Access,
		Target:         access.Target,
	})
	if err != nil {
		return errors.Annotatef(err, "adding permission for user %q", name)
	}

	return nil
}

// AddUserWithPassword adds a new user to the database with the
// provided password hash and salt. If the user already exists an error that
// satisfies usererrors.AlreadyExists will be returned. if the creator does
// not exist that satisfies usererrors.CreatorUUIDNotFound will be returned.
func (s *SharedState) AddUserWithPassword(
	ctx context.Context,
	tx *sqlair.TX,
	uuid user.UUID,
	name string,
	displayName string,
	creatorUUID user.UUID,
	permission permission.AccessSpec,
	passwordHash string,
	salt []byte,
) error {
	err := s.AddUser(ctx, tx, uuid, name, displayName, creatorUUID, permission)
	if err != nil {
		return errors.Annotatef(err, "adding user with uuid %q", uuid)
	}

	return errors.Trace(setPasswordHash(ctx, tx, name, passwordHash, salt))
}
