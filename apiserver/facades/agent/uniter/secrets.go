// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiServerErrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/unitstate"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/rpc/params"
)

// SecretService provides core secrets operations.
type SecretService interface {
	CreateCharmSecret(context.Context, *coresecrets.URI, secret.CreateCharmSecretParams) error
	UpdateCharmSecret(context.Context, *coresecrets.URI, secret.UpdateCharmSecretParams) error
	GetSecretValue(context.Context, *coresecrets.URI, int, secret.SecretAccessor) (coresecrets.SecretValue, *coresecrets.ValueRef, error)
	GrantSecretAccess(context.Context, *coresecrets.URI, secret.SecretAccessParams) error
	GetConsumedRevision(
		ctx context.Context, uri *coresecrets.URI, unitName coreunit.Name,
		refresh, peek bool, labelToUpdate *string) (int, error)

	// CheckSecretManageAccess verifies the unit has RoleManage access on
	// the given secret, including app-owned secrets if the unit is the
	// leader. Returns an error satisfying [secreterrors.PermissionDenied]
	// if access is denied.
	CheckSecretManageAccess(ctx context.Context, uri *coresecrets.URI, unitName coreunit.Name) error

	// GetSecretOwnerKinds returns the owner kind for each of the given
	// secret URIs. Secrets that no longer exist are silently omitted.
	GetSecretOwnerKinds(ctx context.Context, uris []*coresecrets.URI) ([]secret.SecretOwnerInfo, error)

	// ResolveRevokeParams resolves a batch of access params, looking up
	// subject UUIDs. Per-entry errors are in RevokeResult.Error.
	ResolveRevokeParams(ctx context.Context, params []secret.SecretAccessParams) []secret.RevokeResult
}

// createSecrets creates new secrets.
func (u *UniterAPI) createSecrets(ctx context.Context, args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		id, err := u.createSecret(ctx, arg)
		result.Results[i].Result = id
		if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiServerErrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) createSecret(ctx context.Context, arg params.CreateSecretArg) (string, error) {
	if len(arg.Content.Data) == 0 && arg.Content.ValueRef == nil {
		return "", errors.NotValidf("empty secret value")
	}

	authTag := u.auth.GetAuthTag()
	// A unit can only create secrets owned by its app
	// if it is the leader.
	secretOwner, err := names.ParseTag(arg.OwnerTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if !isSameApplication(authTag, secretOwner) {
		return "", apiServerErrors.ErrPerm
	}

	var uri *coresecrets.URI
	if arg.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.URI)
		if err != nil {
			return "", errors.Trace(err)
		}
	} else {
		uri = coresecrets.NewURI()
	}

	params := secret.CreateCharmSecretParams{
		Version: secrets.Version,
		UpdateCharmSecretParams: fromUpsertParams(arg.UpsertSecretArg, secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   authTag.Id(),
		}),
	}
	switch kind := secretOwner.Kind(); kind {
	case names.UnitTagKind:
		params.CharmOwner = secret.CharmSecretOwner{Kind: secret.UnitCharmSecretOwner, ID: secretOwner.Id()}
	case names.ApplicationTagKind:
		params.CharmOwner = secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner, ID: secretOwner.Id()}
	default:
		return "", errors.NotValidf("secret owner kind %q", kind)
	}
	err = u.secretService.CreateCharmSecret(ctx, uri, params)
	if err != nil {
		return "", errors.Trace(err)
	}
	return uri.String(), nil
}

func fromUpsertParams(p params.UpsertSecretArg, accessor secret.SecretAccessor) secret.UpdateCharmSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return secret.UpdateCharmSecretParams{
		Accessor:     accessor,
		RotatePolicy: p.RotatePolicy,
		ExpireTime:   p.ExpireTime,
		Description:  p.Description,
		Label:        p.Label,
		Params:       p.Params,
		Data:         p.Content.Data,
		ValueRef:     valueRef,
		Checksum:     p.Content.Checksum,
	}
}

// updateSecrets updates the specified secrets.
func (u *UniterAPI) updateSecrets(ctx context.Context, args params.UpdateSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := u.updateSecret(ctx, arg)
		if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiServerErrors.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) updateSecret(ctx context.Context, arg params.UpdateSecretArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if arg.RotatePolicy == nil && arg.Description == nil && arg.ExpireTime == nil &&
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Content.Data) == 0 && arg.Content.ValueRef == nil {
		return errors.New("at least one attribute to update must be specified")
	}

	accessor := secret.SecretAccessor{
		Kind: secret.UnitAccessor,
		ID:   u.auth.GetAuthTag().Id(),
	}
	err = u.secretService.UpdateCharmSecret(ctx, uri, fromUpsertParams(arg.UpsertSecretArg, accessor))
	return errors.Trace(err)
}

// isSameApplication returns true if the authenticated entity and the specified entity are in the same application.
func isSameApplication(authTag names.Tag, tag names.Tag) bool {
	return appFromTag(authTag) == appFromTag(tag)
}

func appFromTag(tag names.Tag) string {
	switch tag.Kind() {
	case names.ApplicationTagKind:
		return tag.Id()
	case names.UnitTagKind:
		authAppName, _ := names.UnitApplication(tag.Id())
		return authAppName
	}
	return ""
}

type grantRevokeFunc func(context.Context, *coresecrets.URI, secret.SecretAccessParams) error

// secretsGrant grants access to a secret for the specified subjects.
func (u *UniterAPI) secretsGrant(ctx context.Context, args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return u.secretsGrantRevoke(ctx, args, u.secretService.GrantSecretAccess)
}

func accessorFromTag(tag names.Tag) (secret.SecretAccessor, error) {
	result := secret.SecretAccessor{
		ID: tag.Id(),
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		result.Kind = secret.ApplicationAccessor
	case names.UnitTagKind:
		result.Kind = secret.UnitAccessor
	case names.ModelTagKind:
		result.Kind = secret.ModelAccessor
	default:
		return secret.SecretAccessor{}, errors.Errorf("tag kind %q not valid for secret accessor", kind)
	}
	return result, nil
}

func accessScopeFromTag(tag names.Tag) (secret.SecretAccessScope, error) {
	result := secret.SecretAccessScope{
		ID: tag.Id(),
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		result.Kind = secret.ApplicationAccessScope
	case names.UnitTagKind:
		result.Kind = secret.UnitAccessScope
	case names.RelationTagKind:
		result.Kind = secret.RelationAccessScope
	case names.ModelTagKind:
		result.Kind = secret.ModelAccessScope
	default:
		return secret.SecretAccessScope{}, errors.Errorf("tag kind %q not valid for secret access scope", kind)
	}
	return result, nil
}

func (u *UniterAPI) secretsGrantRevoke(ctx context.Context, args params.GrantRevokeSecretArgs, op grantRevokeFunc) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	one := func(arg params.GrantRevokeSecretArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		var scope secret.SecretAccessScope
		if arg.ScopeTag != "" {
			scopeTag, err := names.ParseTag(arg.ScopeTag)
			if err != nil {
				return errors.Trace(err)
			}
			scope, err = accessScopeFromTag(scopeTag)
			if err != nil {
				return errors.Trace(err)
			}
		}
		role := coresecrets.SecretRole(arg.Role)
		if role != "" && !role.IsValid() {
			return errors.NotValidf("secret role %q", arg.Role)
		}

		accessor := secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   u.auth.GetAuthTag().Id(),
		}

		for _, tagStr := range arg.SubjectTags {
			subjectTag, err := names.ParseTag(tagStr)
			if err != nil {
				return errors.Trace(err)
			}
			subject, err := accessorFromTag(subjectTag)
			if err != nil {
				return errors.Trace(err)
			}
			if err := op(ctx, uri, secret.SecretAccessParams{
				Accessor: accessor,
				Scope:    scope,
				Subject:  subject,
				Role:     role,
			}); err != nil {
				return errors.Annotatef(err, "cannot change access to %q for %q", uri, tagStr)
			}
		}
		return nil
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiServerErrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}

// updateTrackedRevisions updates the consumer info to track the latest
// revisions for the specified secrets.
func (u *UniterAPI) updateTrackedRevisions(ctx context.Context, uris []string) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(uris)),
	}
	authTag := u.auth.GetAuthTag()
	for i, uriStr := range uris {
		uri, err := coresecrets.ParseURI(uriStr)
		if err != nil {
			result.Results[i].Error = apiServerErrors.ServerError(err)
			continue
		}
		unitName, err := coreunit.NewName(authTag.Id())
		if err != nil {
			result.Results[i].Error = apiServerErrors.ServerError(err)
			continue
		}
		_, err = u.secretService.GetConsumedRevision(ctx, uri, unitName, true, false, nil)
		result.Results[i].Error = apiServerErrors.ServerError(err)
	}
	return result, nil
}

// prepareSecretRevokes converts wire-format revoke args to domain types,
// filtering out secrets the unit cannot manage and resolving subject UUIDs
// and ownership.
func (u *UniterAPI) prepareSecretRevokes(
	ctx context.Context, unitName coreunit.Name, revokes []params.GrantRevokeSecretArg,
) ([]unitstate.RevokeSecretArg, error) {
	secretRevokes := make([]unitstate.RevokeSecretArg, 0, len(revokes))
	var revokeErrs []error

	accessor := secret.SecretAccessor{
		Kind: secret.UnitAccessor,
		ID:   u.auth.GetAuthTag().Id(),
	}

	for _, rev := range revokes {
		uri, err := coresecrets.ParseURI(rev.URI)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		if err := u.secretService.CheckSecretManageAccess(ctx, uri, unitName); err != nil {
			if errors.Is(err, secreterrors.SecretNotFound) {
				continue
			}
			revokeErrs = append(revokeErrs, err)
			continue
		}

		args, err := u.resolveRevokeSubjects(ctx, accessor, rev)
		if err != nil {
			return nil, err
		}
		for _, a := range args {
			if a.err != nil {
				revokeErrs = append(revokeErrs, a.err)
				continue
			}
			secretRevokes = append(secretRevokes, unitstate.RevokeSecretArg{
				URI:           uri,
				SubjectUUID:   a.SubjectUUID,
				SubjectTypeID: a.SubjectTypeID,
			})
		}
	}
	if len(revokeErrs) > 0 {
		return nil, internalerrors.Errorf(
			"revoking secrets access: %w", internalerrors.Join(revokeErrs...))
	}

	return u.resolveSecretOwnerKinds(ctx, secretRevokes)
}

// resolveRevokeSubjects resolves the subject tags in a single revoke arg
// into subject UUIDs via the secret service (single bulk call).
func (u *UniterAPI) resolveRevokeSubjects(
	ctx context.Context, accessor secret.SecretAccessor, rev params.GrantRevokeSecretArg,
) ([]revokeSubjectResult, error) {
	var scope secret.SecretAccessScope
	if rev.ScopeTag != "" {
		scopeTag, err := names.ParseTag(rev.ScopeTag)
		if err != nil {
			return nil, err
		}
		scope, err = accessScopeFromTag(scopeTag)
		if err != nil {
			return nil, err
		}
	}
	role := coresecrets.SecretRole(rev.Role)

	// Build the batch of access params.
	accessParams := make([]secret.SecretAccessParams, 0, len(rev.SubjectTags))
	for _, tagStr := range rev.SubjectTags {
		subjectTag, err := names.ParseTag(tagStr)
		if err != nil {
			return nil, err
		}
		subject, err := accessorFromTag(subjectTag)
		if err != nil {
			return nil, err
		}
		accessParams = append(accessParams, secret.SecretAccessParams{
			Accessor: accessor,
			Scope:    scope,
			Subject:  subject,
			Role:     role,
		})
	}

	// Resolve all subjects in one call.
	resolved := u.secretService.ResolveRevokeParams(ctx, accessParams)

	results := make([]revokeSubjectResult, len(resolved))
	for i, r := range resolved {
		if r.Error != nil {
			results[i] = revokeSubjectResult{err: r.Error}
			continue
		}
		results[i] = revokeSubjectResult{
			SubjectUUID:   r.SubjectUUID,
			SubjectTypeID: r.SubjectTypeID,
		}
	}
	return results, nil
}

// revokeSubjectResult holds the resolved subject or an error.
type revokeSubjectResult struct {
	SubjectUUID   string
	SubjectTypeID secret.GrantSubjectType
	err           error
}

// resolveSecretOwnerKinds resolves ownership for a slice of revoke args,
// filtering out secrets that disappeared between the access check and the
// ownership query (deleted concurrently).
func (u *UniterAPI) resolveSecretOwnerKinds(
	ctx context.Context, revokes []unitstate.RevokeSecretArg,
) ([]unitstate.RevokeSecretArg, error) {
	if len(revokes) == 0 {
		return revokes, nil
	}
	uris := make([]*coresecrets.URI, len(revokes))
	for i, r := range revokes {
		uris[i] = r.URI
	}
	ownerInfos, err := u.secretService.GetSecretOwnerKinds(ctx, uris)
	if err != nil {
		return nil, err
	}
	ownerByID := make(map[string]secret.CharmSecretOwnerKind, len(ownerInfos))
	for _, info := range ownerInfos {
		ownerByID[info.SecretID] = info.OwnerKind
	}
	// Filter: secrets that disappeared between the access check and
	// the ownership query were deleted concurrently.
	filtered := revokes[:0]
	for i := range revokes {
		kind, ok := ownerByID[revokes[i].URI.ID]
		if !ok {
			continue
		}
		revokes[i].OwnerKind = kind
		filtered = append(filtered, revokes[i])
	}
	return filtered, nil
}
