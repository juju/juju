// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
)

// GetSecretGrants returns the subjects which have the specified access to the secret.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]SecretAccess, error) {
	accessors, err := s.st.GetSecretGrants(ctx, uri, role)
	if err != nil {
		return nil, errors.Trace(err)
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
func (s *SecretService) GetSecretAccessScope(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) (SecretAccessScope, error) {
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
	accessScope, err := s.st.GetSecretAccessScope(ctx, uri, ap)
	if err != nil {
		return SecretAccessScope{}, errors.Trace(err)
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
	role, err := s.st.GetSecretAccess(ctx, uri, ap)
	if err != nil {
		return secrets.RoleNone, errors.Trace(err)
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
	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
	}

	p := domainsecret.GrantParams{
		ScopeID:   params.Scope.ID,
		SubjectID: params.Subject.ID,
		RoleID:    domainsecret.MarshallRole(params.Role),
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

	switch params.Scope.Kind {
	case UnitAccessScope:
		p.ScopeTypeID = domainsecret.ScopeUnit
	case ApplicationAccessScope:
		p.ScopeTypeID = domainsecret.ScopeApplication
	case ModelAccessScope:
		p.ScopeTypeID = domainsecret.ScopeModel
	case RelationAccessScope:
		p.ScopeTypeID = domainsecret.ScopeRelation
	}

	return s.st.GrantAccess(ctx, uri, p)
}

// RevokeSecretAccess revokes access to the secret for the specified subject.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) RevokeSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) error {
	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
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

	return s.st.RevokeAccess(ctx, uri, p)
}

// canManage checks that the accessor can manage the secret.
// If the request is for a secret owned by an application, the unit must be the leader.
func (s *SecretService) canManage(
	ctx context.Context,
	uri *secrets.URI, assessor SecretAccessor,
	leaderToken leadership.Token,
) error {
	hasRole, err := s.getSecretAccess(ctx, uri, assessor)
	if err != nil {
		// Typically not found error.
		return errors.Trace(err)
	}
	if hasRole.Allowed(secrets.RoleManage) {
		return nil
	}
	// Units can manage app owned secrets if they are the leader.
	if assessor.Kind == UnitAccessor {
		if leaderToken == nil {
			return secreterrors.PermissionDenied
		}
		if err := leaderToken.Check(); err == nil {
			appName, _ := names.UnitApplication(assessor.ID)
			hasRole, err = s.getSecretAccess(ctx, uri, SecretAccessor{
				Kind: ApplicationAccessor,
				ID:   appName,
			})
			if err != nil {
				// Typically not found error.
				return errors.Trace(err)
			}
			if hasRole.Allowed(secrets.RoleManage) {
				return nil
			}
		}
	}
	return fmt.Errorf("%q is not allowed to manage this secret%w", assessor.ID, errors.Hide(secreterrors.PermissionDenied))
}

// canRead checks that the accessor can read the secret.
func (s *SecretService) canRead(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) error {
	// First try looking up unit access.
	hasRole, err := s.getSecretAccess(ctx, uri, accessor)
	if err != nil {
		// Typically not found error.
		return errors.Trace(err)
	}
	if hasRole.Allowed(secrets.RoleView) {
		return nil
	}

	notAllowedErr := fmt.Errorf("%q is not allowed to read this secret%w", accessor.ID, errors.Hide(secreterrors.PermissionDenied))

	if accessor.Kind != UnitAccessor {
		return notAllowedErr
	}
	// All units can read secrets owned by application.
	appName, _ := names.UnitApplication(accessor.ID)
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
		return errors.Trace(err)
	}
	if hasRole.Allowed(secrets.RoleView) {
		return nil
	}
	return notAllowedErr
}
