// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
)

// GetSecretGrants returns the subjects which have the specified access to the secret.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]SecretAccess, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
			// For relation scoped secrets, we need to look up the key from the UUID.
			// TODO - make this a bulk call.
			key, err := s.getRelationKeyByUUID(ctx, accessor.ScopeUUID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			sa.Scope.ID = key.String()
		default:
			// Should never happen.
			return nil, errors.Errorf("unexpected accessor scope type: %#v", accessor.ScopeTypeID)
		}

		result[i] = sa
	}
	return result, nil
}

func (s *SecretService) getRelationKeyByUUID(ctx context.Context, relUUID string) (corerelation.Key, error) {
	endpoints, err := s.secretState.GetRelationEndpoints(ctx, relUUID)
	if err != nil {
		return corerelation.Key{}, errors.Capture(err)
	}
	key, err := corerelation.NewKey(endpoints)
	if err != nil {
		return corerelation.Key{}, errors.Errorf("generating relation key: %w", err)
	}
	return key, nil
}

// GetSecretAccessRelationScope returns the relation UUID of the access scope for the specified
// app or unit's permission on the secret. This requires that the access scope is a relation.
// It returns an error satisfying:
// - [secreterrors.SecretNotFound] if the secret is not found.
// - [secreterrors.SecretAccessScopeNotFound] if the access scope is not found.
func (s *SecretService) GetSecretAccessRelationScope(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) (corerelation.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ap := domainsecret.AccessParams{
		SubjectID: accessor.ID,
	}
	switch accessor.Kind {
	case UnitAccessor:
		ap.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		ap.SubjectTypeID = domainsecret.SubjectApplication
	case ModelAccessor:
		// Should never happen.
		return "", errors.Errorf("getting relation access scope for kind %q not supported", accessor.Kind).Add(coreerrors.NotSupported)
	}
	relationUUID, err := s.secretState.GetSecretAccessRelationScope(ctx, uri, ap)
	if err != nil {
		return "", errors.Capture(err)
	}
	return corerelation.ParseUUID(relationUUID)
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
func (s *SecretService) GrantSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	return withCaveat(ctx, func(innerCtx context.Context) error {
		p, err := s.grantParams(ctx, params)
		if err != nil {
			return errors.Capture(err)
		}
		return s.secretState.GrantAccess(innerCtx, uri, p)
	})
}

func (s *SecretService) getApplicationUUIDByName(ctx context.Context, name string) (string, error) {
	var uuid coreapplication.UUID
	err := s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = s.secretState.GetApplicationUUID(ctx, name)
		return errors.Capture(err)
	})
	return uuid.String(), errors.Capture(err)
}

func (s *SecretService) getUnitUUIDByName(ctx context.Context, name string) (string, error) {
	unitName, err := unit.NewName(name)
	if err != nil {
		return "", errors.Capture(err)
	}
	var uuid unit.UUID
	err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = s.secretState.GetUnitUUID(ctx, unitName)
		return errors.Capture(err)
	})
	return uuid.String(), errors.Capture(err)
}

func (s *SecretService) getRelationUUIDByKey(ctx context.Context, relationKey corerelation.Key) (string, error) {
	if err := relationKey.Validate(); err != nil {
		return "", relationerrors.RelationKeyNotValid
	}

	eids := relationKey.EndpointIdentifiers()
	var uuid string
	var err error
	switch len(eids) {
	case 1:
		return "", errors.Errorf("granting access to a secret over a peer relation is not supported").Add(coreerrors.NotSupported)
	case 2:
		uuid, err = s.secretState.GetRegularRelationUUIDByEndpointIdentifiers(
			ctx,
			eids[0],
			eids[1],
		)
		if err != nil {
			return "", errors.Errorf("getting regular relation by key: %w", err)
		}
		return uuid, nil
	default:
		return "", errors.Errorf("internal error: unexpected number of endpoints %d", len(eids))
	}
}

func (s *SecretService) lookupSubjectUUID(
	ctx context.Context, subjectID string, subjectKind SecretAccessorKind,
) (string, error) {
	switch subjectKind {
	case UnitAccessor:
		return s.getUnitUUIDByName(ctx, subjectID)
	case ApplicationAccessor:
		return s.getApplicationUUIDByName(ctx, subjectID)
	case ModelAccessor:
		// The model ID is the UUID.
		return subjectID, nil
	}
	return "", errors.Errorf("unexpected secret accessor: %s", subjectKind)
}

func (s *SecretService) lookupScopeUUID(
	ctx context.Context, scopeID string, scopeKind SecretAccessScopeKind,
) (string, error) {
	switch scopeKind {
	case UnitAccessScope:
		return s.getUnitUUIDByName(ctx, scopeID)
	case ApplicationAccessScope:
		return s.getApplicationUUIDByName(ctx, scopeID)
	case ModelAccessScope:
		// The model ID is the UUID.
		return scopeID, nil
	case RelationAccessScope:
		key, err := corerelation.NewKeyFromString(scopeID)
		if err != nil {
			return "", errors.Capture(err)
		}
		return s.getRelationUUIDByKey(ctx, key)
	}
	return "", errors.Errorf("unexpected secret access scope: %s", scopeKind)
}

func (s *SecretService) grantParams(ctx context.Context, in SecretAccessParams) (domainsecret.GrantParams, error) {
	scopeUUID, err := s.lookupScopeUUID(ctx, in.Scope.ID, in.Scope.Kind)
	if err != nil {
		return domainsecret.GrantParams{}, errors.Capture(err)
	}
	subjectUUID := scopeUUID
	if string(in.Scope.Kind) != string(in.Subject.Kind) || in.Scope.ID != in.Subject.ID {
		subjectUUID, err = s.lookupSubjectUUID(ctx, in.Subject.ID, in.Subject.Kind)
		if err != nil {
			return domainsecret.GrantParams{}, errors.Capture(err)
		}
	}
	p := domainsecret.GrantParams{
		ScopeUUID:   scopeUUID,
		SubjectUUID: subjectUUID,
		RoleID:      domainsecret.MarshallRole(in.Role),
	}
	switch in.Subject.Kind {
	case UnitAccessor:
		p.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		p.SubjectTypeID = domainsecret.SubjectApplication
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
	return p, nil
}

// RevokeSecretAccess revokes access to the secret for the specified subject.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) RevokeSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	subjectUUID, err := s.lookupSubjectUUID(ctx, params.Subject.ID, params.Subject.Kind)
	if err != nil {
		return errors.Capture(err)
	}
	p := domainsecret.RevokeParams{
		SubjectUUID: subjectUUID,
	}
	switch params.Subject.Kind {
	case UnitAccessor:
		p.SubjectTypeID = domainsecret.SubjectUnit
	case ApplicationAccessor:
		p.SubjectTypeID = domainsecret.SubjectApplication
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
