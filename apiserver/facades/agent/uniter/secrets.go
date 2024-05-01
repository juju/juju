// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiServerErrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// SecretService provides core secrets operations.
type SecretService interface {
	CreateCharmSecret(context.Context, *coresecrets.URI, secretservice.CreateCharmSecretParams) error
	UpdateCharmSecret(context.Context, *coresecrets.URI, secretservice.UpdateCharmSecretParams) error
	DeleteSecret(context.Context, *coresecrets.URI, secretservice.DeleteSecretParams) error
	GetSecretValue(context.Context, *coresecrets.URI, int, secretservice.SecretAccessor) (coresecrets.SecretValue, *coresecrets.ValueRef, error)
	GrantSecretAccess(context.Context, *coresecrets.URI, secretservice.SecretAccessParams) error
	RevokeSecretAccess(context.Context, *coresecrets.URI, secretservice.SecretAccessParams) error
	GetConsumedRevision(
		ctx context.Context, uri *coresecrets.URI, unitName string,
		refresh, peek bool, labelToUpdate *string) (int, error)
}

// createSecrets creates new secrets.
func (u *UniterAPI) createSecrets(ctx context.Context, args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		id, err := u.createSecret(ctx, arg)
		result.Results[i].Result = id
		if errors.Is(err, state.LabelExists) {
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

	appName, _ := names.UnitApplication(authTag.Id())
	token := u.leadershipChecker.LeadershipCheck(appName, authTag.Id())
	var uri *coresecrets.URI
	if arg.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.URI)
		if err != nil {
			return "", errors.Trace(err)
		}
	} else {
		uri = coresecrets.NewURI()
	}

	params := secretservice.CreateCharmSecretParams{
		Version: secrets.Version,
		// Secret accessor not needed when creating a secret since we are using the secret owner.
		UpdateCharmSecretParams: fromUpsertParams(arg.UpsertSecretArg, secretservice.SecretAccessor{}, token),
	}
	switch kind := secretOwner.Kind(); kind {
	case names.UnitTagKind:
		params.CharmOwner = secretservice.CharmSecretOwner{Kind: secretservice.UnitOwner, ID: secretOwner.Id()}
	case names.ApplicationTagKind:
		params.CharmOwner = secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: secretOwner.Id()}
	default:
		return "", errors.NotValidf("secret owner kind %q", kind)
	}
	err = u.secretService.CreateCharmSecret(ctx, uri, params)
	if err != nil {
		return "", errors.Trace(err)
	}
	return uri.String(), nil
}

func fromUpsertParams(p params.UpsertSecretArg, accessor secretservice.SecretAccessor, token leadership.Token) secretservice.UpdateCharmSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return secretservice.UpdateCharmSecretParams{
		LeaderToken:  token,
		Accessor:     accessor,
		RotatePolicy: p.RotatePolicy,
		ExpireTime:   p.ExpireTime,
		Description:  p.Description,
		Label:        p.Label,
		Params:       p.Params,
		Data:         p.Content.Data,
		ValueRef:     valueRef,
	}
}

// updateSecrets updates the specified secrets.
func (u *UniterAPI) updateSecrets(ctx context.Context, args params.UpdateSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := u.updateSecret(ctx, arg)
		if errors.Is(err, state.LabelExists) {
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

	authTag := u.auth.GetAuthTag()
	accessor := secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   authTag.Id(),
	}
	appName, _ := names.UnitApplication(authTag.Id())
	token := u.leadershipChecker.LeadershipCheck(appName, authTag.Id())
	err = u.secretService.UpdateCharmSecret(ctx, uri, fromUpsertParams(arg.UpsertSecretArg, accessor, token))
	return errors.Trace(err)
}

// removeSecrets removes the specified secrets.
func (u *UniterAPI) removeSecrets(ctx context.Context, args params.DeleteSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}

	if len(args.Args) == 0 {
		return result, nil
	}

	authTag := u.auth.GetAuthTag()
	accessor := secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   authTag.Id(),
	}
	appName, _ := names.UnitApplication(authTag.Id())
	token := u.leadershipChecker.LeadershipCheck(appName, authTag.Id())
	for i, arg := range args.Args {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			result.Results[i].Error = apiServerErrors.ServerError(err)
			continue
		}
		p := secretservice.DeleteSecretParams{
			LeaderToken: token,
			Accessor:    accessor,
			Revisions:   arg.Revisions,
		}
		err = u.secretService.DeleteSecret(ctx, uri, p)
		if err != nil {
			result.Results[i].Error = apiServerErrors.ServerError(err)
			continue
		}
	}
	return result, nil
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

type grantRevokeFunc func(context.Context, *coresecrets.URI, secretservice.SecretAccessParams) error

// secretsGrant grants access to a secret for the specified subjects.
func (u *UniterAPI) secretsGrant(ctx context.Context, args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return u.secretsGrantRevoke(ctx, args, u.secretService.GrantSecretAccess)
}

// secretsRevoke revokes access to a secret for the specified subjects.
func (u *UniterAPI) secretsRevoke(ctx context.Context, args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return u.secretsGrantRevoke(ctx, args, u.secretService.RevokeSecretAccess)
}

func accessorFromTag(tag names.Tag) (secretservice.SecretAccessor, error) {
	result := secretservice.SecretAccessor{
		ID: tag.Id(),
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		if strings.HasPrefix(result.ID, "remote-") {
			result.Kind = secretservice.RemoteApplicationAccessor
		} else {
			result.Kind = secretservice.ApplicationAccessor
		}
	case names.UnitTagKind:
		result.Kind = secretservice.UnitAccessor
	case names.ModelTagKind:
		result.Kind = secretservice.ModelAccessor
	default:
		return secretservice.SecretAccessor{}, errors.Errorf("tag kind %q not valid for secret accessor", kind)
	}
	return result, nil
}

func accessScopeFromTag(tag names.Tag) (secretservice.SecretAccessScope, error) {
	result := secretservice.SecretAccessScope{
		ID: tag.Id(),
	}
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		result.Kind = secretservice.ApplicationAccessScope
	case names.UnitTagKind:
		result.Kind = secretservice.UnitAccessScope
	case names.RelationTagKind:
		result.Kind = secretservice.RelationAccessScope
	case names.ModelTagKind:
		result.Kind = secretservice.ModelAccessScope
	default:
		return secretservice.SecretAccessScope{}, errors.Errorf("tag kind %q not valid for secret access scope", kind)
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
		var scope secretservice.SecretAccessScope
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

		authTag := u.auth.GetAuthTag()
		accessor := secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   authTag.Id(),
		}
		appName, _ := names.UnitApplication(authTag.Id())
		token := u.leadershipChecker.LeadershipCheck(appName, authTag.Id())

		for _, tagStr := range arg.SubjectTags {
			subjectTag, err := names.ParseTag(tagStr)
			if err != nil {
				return errors.Trace(err)
			}
			subject, err := accessorFromTag(subjectTag)
			if err != nil {
				return errors.Trace(err)
			}
			if err := op(ctx, uri, secretservice.SecretAccessParams{
				LeaderToken: token,
				Accessor:    accessor,
				Scope:       scope,
				Subject:     subject,
				Role:        role,
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
		_, err = u.secretService.GetConsumedRevision(ctx, uri, authTag.Id(), true, false, nil)
		result.Results[i].Error = apiServerErrors.ServerError(err)
	}
	return result, nil
}
