// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ExternalUserImport is a public alias for the domain-internal
// [internal.ExternalUserImport] shape, so that callers outside the access
// domain (e.g. the v8 migration import driver in internal/migration, which
// cannot import domain/access/internal) can still build the argument to
// [UserService.ImportExternalUsers].
type ExternalUserImport = internal.ExternalUserImport

// ImportExternalUsers creates external users from a migrated model on the
// target controller. Each user is created with [corepermission.EveryoneUserName]
// as creator (consistent with how external users are created on first
// authentication) and with their original creation date. If a user already
// exists on the target controller, their creation is silently skipped.
// Permission granting and last-login tracking are handled separately by the
// migration operation.
// The following error types are possible from this function:
//   - [accesserrors.UserNotFound]: If [corepermission.EveryoneUserName] does
//     not exist on the controller.
func (s *UserService) ImportExternalUsers(
	ctx context.Context,
	users []ExternalUserImport,
) error {
	if len(users) == 0 {
		return nil
	}

	everyoneUUID, err := s.GetUserUUIDByName(ctx, corepermission.EveryoneUserName)
	if err != nil {
		return errors.Errorf(
			"getting %q UUID: %w", corepermission.EveryoneUserName, err,
		)
	}

	for _, u := range users {
		userUUID, err := user.NewUUID()
		if err != nil {
			return errors.Errorf("generating UUID for user %q: %w", u.Name, err)
		}

		err = s.st.AddUserWithCreatedAt(
			ctx, userUUID, u.Name, u.DisplayName, everyoneUUID, u.DateCreated,
		)
		if err != nil && !errors.Is(err, accesserrors.UserAlreadyExists) {
			return errors.Errorf("adding external user %q: %w", u.Name, err)
		}
	}

	return nil
}

// ImportModelUsers creates any controller user referenced by the migrated
// model that is missing on the target: external users via a single batched
// ImportExternalUsers call. Local users are never created here: the legacy
// import path requires them to already exist on the target (no password
// material travels with a migration), and v8 keeps that constraint.
//
// [UserService.GetUserByName] never returns a removed user, so "found" here
// always means an active target row; per the target-wins rule, such a user is
// left completely alone regardless of what the source says about it. A user
// not found is either missing outright or exists but disabled on the target;
// for an external user that the source also marks removed, this resolves by
// creating then immediately disabling it, so it satisfies provenance/FK
// references without carrying live auth state — for every other not-found case
// (a local user, or a name colliding with a pre-existing disabled target row)
// there is no safe way to make it active, so it is left unresolved for later
// steps to skip.
//
// The returned set holds the usernames that do not have an active target
// identity after this call, so later import steps can skip granting them fresh
// permission/key/login state.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *UserService) ImportModelUsers(
	ctx context.Context, users []coremodelmigration.ModelUser,
) (set.Strings, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	inactive := set.NewStrings()
	if len(users) == 0 {
		return inactive, nil
	}

	var externalImports []ExternalUserImport
	var toDisable []user.Name

	for _, u := range users {
		name, err := user.NewName(u.Name)
		if err != nil {
			return nil, errors.Errorf("invalid username %q: %w", u.Name, err)
		}

		if _, err := s.GetUserByName(ctx, name); err == nil {
			continue
		} else if !errors.Is(err, accesserrors.UserNotFound) {
			return nil, errors.Errorf("looking up user %q: %w", u.Name, err)
		}

		if !u.External {
			inactive.Add(u.Name)
			continue
		}

		externalImports = append(externalImports, ExternalUserImport{
			Name:        name,
			DisplayName: u.DisplayName,
			DateCreated: u.CreatedAt,
		})
		if u.Removed {
			toDisable = append(toDisable, name)
			inactive.Add(u.Name)
		}
	}

	if err := s.ImportExternalUsers(ctx, externalImports); err != nil {
		return nil, errors.Errorf("importing external users: %w", err)
	}

	for _, name := range toDisable {
		if err := s.RemoveUser(ctx, name); err != nil {
			return nil, errors.Errorf("marking user %q removed: %w", name, err)
		}
	}

	return inactive, nil
}

// ImportLastModelLogins records each user's last login time against the model,
// skipping users in inactiveUsers (see [UserService.ImportModelUsers]) and
// users who never logged in.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *UserService) ImportLastModelLogins(
	ctx context.Context,
	modelUUID coremodel.UUID,
	users []coremodelmigration.ModelUser,
	inactiveUsers set.Strings,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(users) == 0 {
		return nil
	}

	for _, u := range users {
		if u.LastLogin == nil || inactiveUsers.Contains(u.Name) {
			continue
		}
		name, err := user.NewName(u.Name)
		if err != nil {
			return errors.Errorf("invalid username %q: %w", u.Name, err)
		}
		if err := s.SetLastModelLogin(ctx, name, modelUUID, *u.LastLogin); err != nil {
			return errors.Errorf("setting last login for %q: %w", u.Name, err)
		}
	}
	return nil
}

// ImportModelPermissions writes the model and offer permission grants carried
// by the envelope. Model grants are written individually; offer grants are
// grouped by offer UUID and written in a single ImportOfferAccess call. Users
// in inactiveUsers (see [UserService.ImportModelUsers]) are skipped: they have
// no active target identity to grant live permission state to. It returns the
// offer UUIDs granted, for the caller to record against the import claim.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *PermissionService) ImportModelPermissions(
	ctx context.Context,
	perms []coremodelmigration.ModelPermission,
	inactiveUsers set.Strings,
) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(perms) == 0 {
		return nil, nil
	}

	offerAccess := make(map[string]map[string]corepermission.Access)
	var offerUUIDs []string

	for _, p := range perms {
		if inactiveUsers.Contains(p.SubjectName) {
			continue
		}

		switch corepermission.ObjectType(p.ObjectType) {
		case corepermission.Model:
			subject, err := user.NewName(p.SubjectName)
			if err != nil {
				return nil, errors.Errorf("invalid permission subject %q: %w", p.SubjectName, err)
			}
			if _, err := s.CreatePermission(ctx, corepermission.UserAccessSpec{
				AccessSpec: corepermission.AccessSpec{
					Access: corepermission.Access(p.Access),
					Target: corepermission.ID{ObjectType: corepermission.Model, Key: p.GrantOn},
				},
				User: subject,
			}); err != nil {
				return nil, errors.Errorf(
					"granting %q access to %q on model: %w", p.Access, p.SubjectName, err)
			}
		case corepermission.Offer:
			if _, ok := offerAccess[p.GrantOn]; !ok {
				offerUUIDs = append(offerUUIDs, p.GrantOn)
				offerAccess[p.GrantOn] = make(map[string]corepermission.Access)
			}
			offerAccess[p.GrantOn][p.SubjectName] = corepermission.Access(p.Access)
		default:
			return nil, errors.Errorf("unknown permission object type %q", p.ObjectType)
		}
	}

	if len(offerUUIDs) > 0 {
		imports := make([]access.OfferImportAccess, 0, len(offerUUIDs))
		for _, offerUUID := range offerUUIDs {
			parsed, err := uuid.UUIDFromString(offerUUID)
			if err != nil {
				return nil, errors.Errorf("invalid offer uuid %q: %w", offerUUID, err)
			}
			imports = append(imports, access.OfferImportAccess{UUID: parsed, Access: offerAccess[offerUUID]})
		}
		if err := s.ImportOfferAccess(ctx, imports); err != nil {
			return nil, errors.Errorf("importing offer permissions: %w", err)
		}
	}

	return offerUUIDs, nil
}
