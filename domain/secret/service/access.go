// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
)

// GetSecretGrants returns the subjects which have the specified access to the secret.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) (_ []SecretAccess, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	accessors, err := s.secretState.GetSecretGrants(ctx, uri, role)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make([]SecretAccess, len(accessors))
	for i, accessor := range accessors {
		sa := SecretAccess{Role: role}
		sa.Subject.ID = accessor.SubjectID
		switch accessor.SubjectTypeID {
		case domainsecret.SubjectUnit:
			sa.Subject.Kind = UnitAccessor
		case domainsecret.SubjectApplication:
			sa.Subject.Kind = ApplicationAccessor
		case domainsecret.SubjectModel:
			sa.Subject.Kind = ModelAccessor
		default:
			// Should never happen.
			return nil, errors.Errorf("unexpected accessor subject type: %#v", accessor.SubjectTypeID)
		}

		sa.Scope.ID = accessor.ScopeID
		switch accessor.ScopeTypeID {
		case domainsecret.ScopeUnit:
			sa.Scope.Kind = UnitAccessScope
		case domainsecret.ScopeApplication:
			sa.Scope.Kind = ApplicationAccessScope
		case domainsecret.ScopeModel:
			sa.Scope.Kind = ModelAccessScope
		case domainsecret.ScopeRelation:
			sa.Scope.Kind = RelationAccessScope
		default:
			// Should never happen.
			return nil, errors.Errorf("unexpected accessor scope type: %#v", accessor.ScopeTypeID)
		}

		result[i] = sa
	}
	return result, nil
}

// GetSecretAccessScope returns the access scope for the specified accessor's permission on the secret.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) GetSecretAccessScope(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) (_ SecretAccessScope, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	ap := domainsecret.AccessParams{
		SubjectID: accessor.ID,
	}
	switch accessor.Kind {
	case UnitAccessor:
		ap.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		ap.SubjectTypeID = domainsecret.SubjectApplication
	case RemoteApplicationAccessor:
		ap.SubjectTypeID = domainsecret.SubjectRemoteApplication
	case ModelAccessor:
		ap.SubjectTypeID = domainsecret.SubjectModel
	}
	accessScope, err := s.secretState.GetSecretAccessScope(ctx, uri, ap)
	if err != nil {
		return SecretAccessScope{}, errors.Capture(err)
	}
	result := SecretAccessScope{
		ID: accessScope.ScopeID,
	}
	switch accessScope.ScopeTypeID {
	case domainsecret.ScopeUnit:
		result.Kind = UnitAccessScope
	case domainsecret.ScopeApplication:
		result.Kind = ApplicationAccessScope
	case domainsecret.ScopeModel:
		result.Kind = ModelAccessScope
	case domainsecret.ScopeRelation:
		result.Kind = RelationAccessScope
	}
	return result, nil
}

// getSecretAccess returns the access to the secret for the specified accessor.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) getSecretAccess(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) (secrets.SecretRole, error) {
	ap := domainsecret.AccessParams{
		SubjectID: accessor.ID,
	}
	switch accessor.Kind {
	case UnitAccessor:
		ap.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		ap.SubjectTypeID = domainsecret.SubjectApplication
	case RemoteApplicationAccessor:
		ap.SubjectTypeID = domainsecret.SubjectRemoteApplication
	case ModelAccessor:
		ap.SubjectTypeID = domainsecret.SubjectModel
	}
	role, err := s.secretState.GetSecretAccess(ctx, uri, ap)
	if err != nil {
		return secrets.RoleNone, errors.Capture(err)
	}
	// "none" is db value, secret enum is "".
	if role == "none" {
		return secrets.RoleNone, nil
	}
	return secrets.SecretRole(role), nil
}

// GrantSecretAccess grants access to the secret for the specified subject with the specified scope.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
// If an attempt is made to change an existing permission's scope or subject type, an error
// satisfying [secreterrors.InvalidSecretPermissionChange] is returned.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) GrantSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	return withCaveat(ctx, func(innerCtx context.Context) error {
		return s.secretState.GrantAccess(innerCtx, uri, grantParams(params))
	})
}

func grantParams(in SecretAccessParams) domainsecret.GrantParams {
	p := domainsecret.GrantParams{
		ScopeID:   in.Scope.ID,
		SubjectID: in.Subject.ID,
		RoleID:    domainsecret.MarshallRole(in.Role),
	}
	switch in.Subject.Kind {
	case UnitAccessor:
		p.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		p.SubjectTypeID = domainsecret.SubjectApplication
	case RemoteApplicationAccessor:
		p.SubjectTypeID = domainsecret.SubjectRemoteApplication
	case ModelAccessor:
		p.SubjectTypeID = domainsecret.SubjectModel
	}

	switch in.Scope.Kind {
	case UnitAccessScope:
		p.ScopeTypeID = domainsecret.ScopeUnit
	case ApplicationAccessScope:
		p.ScopeTypeID = domainsecret.ScopeApplication
	case ModelAccessScope:
		p.ScopeTypeID = domainsecret.ScopeModel
	case RelationAccessScope:
		p.ScopeTypeID = domainsecret.ScopeRelation
	}
	return p
}

// RevokeSecretAccess revokes access to the secret for the specified subject.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) RevokeSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	p := domainsecret.AccessParams{
		SubjectID: params.Subject.ID,
	}
	switch params.Subject.Kind {
	case UnitAccessor:
		p.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		p.SubjectTypeID = domainsecret.SubjectApplication
	case RemoteApplicationAccessor:
		p.SubjectTypeID = domainsecret.SubjectRemoteApplication
	case ModelAccessor:
		p.SubjectTypeID = domainsecret.SubjectModel
	}

	return withCaveat(ctx, func(innerCtx context.Context) error {
		return s.secretState.RevokeAccess(innerCtx, uri, p)
	})
}

// getManagementCaveat returns a function within which an operation can be
// executed if the caveat remains satisfied.
// If the secret is unit-owned and the unit can manage it, the caveat is always
// permissive.
// If the secret is application-owned, the unit must be, and remain the leader
// of that application.
// If the caveat can never be satisfied, an error is returned - the input
// accessor can never manage the input secret.
func (s *SecretService) getManagementCaveat(
	ctx context.Context, uri *secrets.URI, accessor SecretAccessor,
) (func(context.Context, func(context.Context) error) error, error) {
	hasRole, err := s.getSecretAccess(ctx, uri, accessor)
	if err != nil {
		// Typically not found error.
		return nil, errors.Capture(err)
	}
	if hasRole.Allowed(secrets.RoleManage) {
		return func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		}, nil
	}
	// Units can manage app owned secrets if they are the leader.
	if accessor.Kind == UnitAccessor {
		unitName, err := unit.NewName(accessor.ID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		appName := unitName.Application()
		if err := s.leaderEnsurer.LeadershipCheck(appName, accessor.ID).Check(); err == nil {
			hasRole, err = s.getSecretAccess(ctx, uri, SecretAccessor{
				Kind: ApplicationAccessor,
				ID:   appName,
			})
			if err != nil {
				// Typically not found error.
				return nil, errors.Capture(err)
			}

			if hasRole.Allowed(secrets.RoleManage) {
				return func(ctx context.Context, fn func(context.Context) error) error {
					return s.leaderEnsurer.WithLeader(ctx, appName, accessor.ID, fn)
				}, nil
			}
		}
	}
	return nil, errors.Errorf(
		"%q is not allowed to manage this secret", accessor.ID).Add(secreterrors.PermissionDenied)

}

// canRead checks that the accessor can read the secret.
func (s *SecretService) canRead(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) error {
	// First try looking up unit access.
	hasRole, err := s.getSecretAccess(ctx, uri, accessor)
	if err != nil {
		// Typically not found error.
		return errors.Capture(err)
	}
	if hasRole.Allowed(secrets.RoleView) {
		return nil
	}

	notAllowedErr := errors.Errorf("%q is not allowed to read this secret", accessor.ID).Add(secreterrors.PermissionDenied)

	if accessor.Kind != UnitAccessor {
		return notAllowedErr
	}
	// All units can read secrets owned by application.
	unitName, err := unit.NewName(accessor.ID)
	if err != nil {
		return errors.Capture(err)
	}
	appName := unitName.Application()
	kind := ApplicationAccessor
	// Remote apps need a different accessor kind.
	if strings.HasPrefix(appName, "remote-") {
		kind = RemoteApplicationAccessor
	}

	hasRole, err = s.getSecretAccess(ctx, uri, SecretAccessor{
		Kind: kind,
		ID:   appName,
	})
	if err != nil {
		// Typically not found error.
		return errors.Capture(err)
	}
	if hasRole.Allowed(secrets.RoleView) {
		return nil
	}
	return notAllowedErr
}
