// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
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

// GetSecretAccess returns the access to the secret for the specified accessor.
// It returns an error satisfying [secreterrors.SecretNotFound] if the secret is not found.
func (s *SecretService) GetSecretAccess(ctx context.Context, uri *secrets.URI, accessor SecretAccessor) (secrets.SecretRole, error) {
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
func (s *SecretService) GrantSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) error {
	if params.LeaderToken != nil {
		if err := params.LeaderToken.Check(); err != nil {
			return errors.Trace(err)
		}
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
	if params.LeaderToken != nil {
		if err := params.LeaderToken.Check(); err != nil {
			return errors.Trace(err)
		}
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
