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

	// CreateCharmSecret creates a new charm secret with the specified
	// parameters and associates it with a given URI.
	CreateCharmSecret(context.Context, *coresecrets.URI, secret.CreateCharmSecretParams) error

	// UpdateCharmSecret updates an existing charm secret with the provided
	// parameters and returns an error if the operation fails.
	UpdateCharmSecret(context.Context, *coresecrets.URI, secret.UpdateCharmSecretParams) error

	// GetSecretValue retrieves the value and reference of a secret for a
	// specified URI and revision, using a secret accessor.
	GetSecretValue(context.Context, *coresecrets.URI, int, secret.SecretAccessor) (coresecrets.SecretValue, *coresecrets.ValueRef, error)

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

	// ResolveGrantParams resolves a batch of access params, looking up
	// subject and scope UUIDs. Per-entry errors are in GrantResult.Error.
	ResolveGrantParams(ctx context.Context, params []secret.SecretAccessParams) []secret.GrantResult
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
		Data:         p.Content.Data,
		ValueRef:     valueRef,
		Checksum:     p.Content.Checksum,
	}
}

// prepareSecretCreates validates and converts a list of create args from the
// wire format into domain types ready for CommitHookChanges. Per-entry
// validation errors are collected; only structural URI parse errors cause
// an immediate abort.
func (u *UniterAPI) prepareSecretCreates(
	ctx context.Context, creates []params.CreateSecretArg,
) ([]unitstate.CreateSecretArg, error) {
	authTag := u.auth.GetAuthTag()
	secretCreates := make([]unitstate.CreateSecretArg, 0, len(creates))
	var createErrs []error
	for _, createArg := range creates {
		if len(createArg.Content.Data) == 0 && createArg.Content.ValueRef == nil {
			createErrs = append(createErrs, errors.NotValidf("empty secret value"))
			continue
		}
		secretOwner, err := names.ParseTag(createArg.OwnerTag)
		if err != nil {
			createErrs = append(createErrs, err)
			continue
		}
		if !isSameApplication(authTag, secretOwner) {
			createErrs = append(createErrs, apiServerErrors.ErrPerm)
			continue
		}

		createParams := secret.CreateCharmSecretParams{
			Version: secrets.Version,
			UpdateCharmSecretParams: fromUpsertParams(createArg.UpsertSecretArg, secret.SecretAccessor{
				Kind: secret.UnitAccessor,
				ID:   authTag.Id(),
			}),
		}
		switch kind := secretOwner.Kind(); kind {
		case names.UnitTagKind:
			createParams.CharmOwner = secret.CharmSecretOwner{Kind: secret.UnitCharmSecretOwner, ID: secretOwner.Id()}
		case names.ApplicationTagKind:
			createParams.CharmOwner = secret.CharmSecretOwner{Kind: secret.ApplicationCharmSecretOwner, ID: secretOwner.Id()}
		default:
			createErrs = append(createErrs, errors.NotValidf("secret owner kind %q", kind))
			continue
		}
		var uri *coresecrets.URI
		if createArg.URI != nil {
			uri, err = coresecrets.ParseURI(*createArg.URI)
			if err != nil {
				createErrs = append(createErrs, err)
				continue
			}
		} else {
			uri = coresecrets.NewURI()
		}
		secretCreates = append(secretCreates, unitstate.CreateSecretArg{
			CreateCharmSecretParams: createParams,
			URI:                     uri,
		})
	}
	if len(createErrs) > 0 {
		return nil, internalerrors.Errorf(
			"creating secrets: %w", internalerrors.Join(createErrs...))
	}
	return secretCreates, nil
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

// prepareSecretTrackLatest validates the URI strings for secrets whose latest
// revision the unit wants to track. The input strings may be full URI strings
// (secret:<id>) or bare secret IDs (xid format); ParseURI accepts both. The
// function validates each URI and returns the corresponding IDs ready to be
// passed into the CommitHookChanges transaction.
func (u *UniterAPI) prepareSecretTrackLatest(uris []string) ([]string, error) {
	result := make([]string, 0, len(uris))
	for _, uriStr := range uris {
		uri, err := coresecrets.ParseURI(uriStr)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		result = append(result, uri.ID)
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
				u.logger.Infof(ctx, "secret %q no longer exists, skipping revoke", rev.URI)
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
	if role != "" && !role.IsValid() {
		return nil, errors.NotValidf("secret role %q", rev.Role)
	}

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

// prepareSecretGrants converts wire-format grant args to domain types,
// filtering out secrets the unit cannot manage and resolving subject/scope
// UUIDs and ownership.
func (u *UniterAPI) prepareSecretGrants(
	ctx context.Context, unitName coreunit.Name, grants []params.GrantRevokeSecretArg,
) ([]unitstate.GrantSecretArg, error) {
	secretGrants := make([]unitstate.GrantSecretArg, 0, len(grants))
	var grantErrs []error

	accessor := secret.SecretAccessor{
		Kind: secret.UnitAccessor,
		ID:   u.auth.GetAuthTag().Id(),
	}

	for _, g := range grants {
		uri, err := coresecrets.ParseURI(g.URI)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		if err := u.secretService.CheckSecretManageAccess(ctx, uri, unitName); err != nil {
			if errors.Is(err, secreterrors.SecretNotFound) {
				u.logger.Infof(ctx, "secret %q no longer exists, skipping grant", g.URI)
				continue
			}
			grantErrs = append(grantErrs, err)
			continue
		}

		args, err := u.resolveGrantSubjects(ctx, accessor, g)
		if err != nil {
			return nil, err
		}
		for _, a := range args {
			if a.err != nil {
				grantErrs = append(grantErrs, a.err)
				continue
			}
			secretGrants = append(secretGrants, unitstate.GrantSecretArg{
				URI:           uri,
				SubjectUUID:   a.SubjectUUID,
				SubjectTypeID: a.SubjectTypeID,
				ScopeUUID:     a.ScopeUUID,
				ScopeTypeID:   a.ScopeTypeID,
				RoleID:        a.RoleID,
			})
		}
	}
	if len(grantErrs) > 0 {
		return nil, internalerrors.Errorf(
			"granting secrets access: %w", internalerrors.Join(grantErrs...))
	}

	return u.resolveGrantOwnerKinds(ctx, secretGrants)
}

// grantSubjectResult holds the resolved grant params or an error.
type grantSubjectResult struct {
	SubjectUUID   string
	SubjectTypeID secret.GrantSubjectType
	ScopeUUID     string
	ScopeTypeID   secret.GrantScopeType
	RoleID        secret.Role
	err           error
}

// resolveGrantSubjects resolves the subject and scope tags in a single grant
// arg into UUIDs via the secret service (single bulk call).
func (u *UniterAPI) resolveGrantSubjects(
	ctx context.Context, accessor secret.SecretAccessor, g params.GrantRevokeSecretArg,
) ([]grantSubjectResult, error) {
	var scope secret.SecretAccessScope
	if g.ScopeTag != "" {
		scopeTag, err := names.ParseTag(g.ScopeTag)
		if err != nil {
			return nil, err
		}
		scope, err = accessScopeFromTag(scopeTag)
		if err != nil {
			return nil, err
		}
	}
	role := coresecrets.SecretRole(g.Role)
	if role != "" && !role.IsValid() {
		return nil, errors.NotValidf("secret role %q", g.Role)
	}

	// Build the batch of access params.
	accessParams := make([]secret.SecretAccessParams, 0, len(g.SubjectTags))
	for _, tagStr := range g.SubjectTags {
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
	resolved := u.secretService.ResolveGrantParams(ctx, accessParams)

	results := make([]grantSubjectResult, len(resolved))
	for i, r := range resolved {
		if r.Error != nil {
			results[i] = grantSubjectResult{err: r.Error}
			continue
		}
		results[i] = grantSubjectResult{
			SubjectUUID:   r.SubjectUUID,
			SubjectTypeID: r.SubjectTypeID,
			ScopeUUID:     r.ScopeUUID,
			ScopeTypeID:   r.ScopeTypeID,
			RoleID:        r.RoleID,
		}
	}
	return results, nil
}

// resolveGrantOwnerKinds resolves ownership for a slice of grant args,
// filtering out secrets that disappeared between the access check and the
// ownership query (deleted concurrently).
func (u *UniterAPI) resolveGrantOwnerKinds(
	ctx context.Context, grants []unitstate.GrantSecretArg,
) ([]unitstate.GrantSecretArg, error) {
	if len(grants) == 0 {
		return grants, nil
	}
	uris := make([]*coresecrets.URI, len(grants))
	for i, g := range grants {
		uris[i] = g.URI
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
	filtered := grants[:0]
	for i := range grants {
		kind, ok := ownerByID[grants[i].URI.ID]
		if !ok {
			continue
		}
		grants[i].OwnerKind = kind
		filtered = append(filtered, grants[i])
	}
	return filtered, nil
}

// prepareSecretDeletes validates and resolves a list of delete args from the
// wire format into domain types ready for CommitHookChanges. It:
//   - parses the URI
//   - checks manage access (skips SecretNotFound, accumulates other errors)
//   - resolves ownership via GetSecretOwnerKinds (filters secrets that
//     disappeared between the access check and the ownership query)
func (u *UniterAPI) prepareSecretDeletes(
	ctx context.Context, unitName coreunit.Name, deletes []params.DeleteSecretArg,
) ([]unitstate.DeleteSecretArg, error) {
	secretDeletes := make([]unitstate.DeleteSecretArg, 0, len(deletes))
	var deleteErrs []error

	for _, del := range deletes {
		uri, err := coresecrets.ParseURI(del.URI)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		if err := u.secretService.CheckSecretManageAccess(ctx, uri, unitName); err != nil {
			if errors.Is(err, secreterrors.SecretNotFound) {
				u.logger.Infof(ctx, "secret %q no longer exists, skipping delete", del.URI)
				continue
			}
			deleteErrs = append(deleteErrs, err)
			continue
		}
		secretDeletes = append(secretDeletes, unitstate.DeleteSecretArg{
			URI: uri,
			DeleteSecretParams: secret.DeleteSecretParams{
				Revisions: del.Revisions,
			},
		})
	}
	if len(deleteErrs) > 0 {
		return nil, internalerrors.Errorf(
			"removing secrets: %w", internalerrors.Join(deleteErrs...))
	}

	return u.resolveDeleteOwnerKinds(ctx, secretDeletes)
}

// resolveDeleteOwnerKinds resolves ownership for a slice of delete args,
// filtering out secrets that disappeared between the access check and the
// ownership query (deleted concurrently).
func (u *UniterAPI) resolveDeleteOwnerKinds(
	ctx context.Context, deletes []unitstate.DeleteSecretArg,
) ([]unitstate.DeleteSecretArg, error) {
	if len(deletes) == 0 {
		return deletes, nil
	}
	uris := make([]*coresecrets.URI, len(deletes))
	for i, d := range deletes {
		uris[i] = d.URI
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
	filtered := deletes[:0]
	for i := range deletes {
		kind, ok := ownerByID[deletes[i].URI.ID]
		if !ok {
			continue
		}
		deletes[i].OwnerKind = kind
		filtered = append(filtered, deletes[i])
	}
	return filtered, nil
}

// prepareSecretUpdates validates and resolves a list of update args from the
// wire format into domain types ready for CommitHookChanges. It:
//   - parses the URI
//   - checks manage access (skips SecretNotFound, accumulates other errors)
//   - resolves ownership via GetSecretOwnerKinds (filters secrets that
//     disappeared between the access check and the ownership query)
func (u *UniterAPI) prepareSecretUpdates(
	ctx context.Context, unitName coreunit.Name, updates []params.UpdateSecretArg,
) ([]unitstate.UpdateSecretArg, error) {
	secretUpdates := make([]unitstate.UpdateSecretArg, 0, len(updates))
	var updateErrs []error

	for _, upd := range updates {
		uri, err := coresecrets.ParseURI(upd.URI)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		if err := u.secretService.CheckSecretManageAccess(ctx, uri, unitName); err != nil {
			if errors.Is(err, secreterrors.SecretNotFound) {
				u.logger.Infof(ctx, "secret %q no longer exists, skipping update", upd.URI)
				continue
			}
			updateErrs = append(updateErrs, err)
			continue
		}
		if !upd.HasUpdate() {
			updateErrs = append(updateErrs, errors.New("at least one attribute to update must be specified"))
			continue
		}
		var valueRef *coresecrets.ValueRef
		if upd.Content.ValueRef != nil {
			valueRef = &coresecrets.ValueRef{
				BackendID:  upd.Content.ValueRef.BackendID,
				RevisionID: upd.Content.ValueRef.RevisionID,
			}
		}
		if len(upd.Params) > 0 {
			u.logger.Warningf(ctx, "params provided for secret %q are ignored (params are not supported in charm secrets)", upd.URI)
		}
		secretUpdates = append(secretUpdates, unitstate.UpdateSecretArg{
			UpdateCharmSecretParams: secret.UpdateCharmSecretParams{
				RotatePolicy: upd.RotatePolicy,
				ExpireTime:   upd.ExpireTime,
				Description:  upd.Description,
				Label:        upd.Label,
				Data:         upd.Content.Data,
				ValueRef:     valueRef,
				Checksum:     upd.Content.Checksum,
			},
			URI: uri,
		})
	}
	if len(updateErrs) > 0 {
		return nil, internalerrors.Errorf(
			"updating secrets: %w", internalerrors.Join(updateErrs...))
	}

	return u.resolveUpdateOwnerKinds(ctx, secretUpdates)
}

// resolveUpdateOwnerKinds populates OwnerKind for each update arg by batch
// querying secret ownership, filtering out secrets that disappeared between
// the access check and the ownership query (deleted concurrently).
func (u *UniterAPI) resolveUpdateOwnerKinds(
	ctx context.Context, updates []unitstate.UpdateSecretArg,
) ([]unitstate.UpdateSecretArg, error) {
	if len(updates) == 0 {
		return updates, nil
	}
	uris := make([]*coresecrets.URI, len(updates))
	for i, upd := range updates {
		uris[i] = upd.URI
	}
	ownerInfos, err := u.secretService.GetSecretOwnerKinds(ctx, uris)
	if err != nil {
		return nil, err
	}
	ownerByID := make(map[string]secret.CharmSecretOwnerKind, len(ownerInfos))
	for _, info := range ownerInfos {
		ownerByID[info.SecretID] = info.OwnerKind
	}
	filtered := updates[:0]
	for _, update := range updates {
		kind, ok := ownerByID[update.URI.ID]
		if !ok {
			continue
		}
		update.OwnerKind = kind
		filtered = append(filtered, update)
	}
	return filtered, nil
}
