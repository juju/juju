// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.secretsmanager")

// SecretsManagerAPI is the implementation for the SecretsManager facade.
type SecretsManagerAPI struct {
	controllerUUID string
	modelUUID      string

	leadershipChecker leadership.Checker
	secretsService    secrets.SecretsService
	resources         facade.Resources
	secretsRotation   SecretsRotation
	secretsConsumer   SecretsConsumer
	authTag           names.Tag
	clock             clock.Clock
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		ID, err := s.createSecret(ctx, arg)
		result.Results[i].Result = ID
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) createSecret(ctx context.Context, arg params.CreateSecretArg) (string, error) {
	if len(arg.Data) == 0 {
		return "", errors.NotValidf("empty secret value")
	}
	// A unit can only create secrets owned by its app.
	secretOwner, err := names.ParseTag(arg.OwnerTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	// A unit can create a secret so long as the
	// secret owner is that unit's app.
	appName := authTagApp(s.authTag)
	if appName != secretOwner.Id() {
		return "", apiservererrors.ErrPerm
	}
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(0, nil); err != nil {
		return "", errors.Trace(err)
	}
	uri := coresecrets.NewURI()
	var nextRotateTime *time.Time
	if arg.RotatePolicy.WillRotate() {
		nextRotateTime = arg.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	md, err := s.secretsService.CreateSecret(ctx, uri, secrets.CreateParams{
		Version:      secrets.Version,
		Owner:        arg.OwnerTag,
		UpsertParams: fromUpsertParams(arg.UpsertSecretArg, token, nextRotateTime),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	err = s.secretsConsumer.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: token,
		Scope:       secretOwner,
		Subject:     secretOwner,
		Role:        coresecrets.RoleManage,
	})
	if err != nil {
		// TODO(wallyworld) - remove secret when that is supported
		return "", errors.Annotate(err, "granting secret owner permission to manage the secret")
	}
	return md.URI.ShortString(), nil
}

func fromUpsertParams(p params.UpsertSecretArg, token leadership.Token, nextRotateTime *time.Time) secrets.UpsertParams {
	return secrets.UpsertParams{
		LeaderToken:    token,
		RotatePolicy:   p.RotatePolicy,
		NextRotateTime: nextRotateTime,
		ExpireTime:     p.ExpireTime,
		Description:    p.Description,
		Label:          p.Label,
		Params:         p.Params,
		Data:           p.Data,
	}
}

// UpdateSecrets updates the specified secrets.
func (s *SecretsManagerAPI) UpdateSecrets(args params.UpdateSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) updateSecret(ctx context.Context, arg params.UpdateSecretArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if uri.ControllerUUID != "" && uri.ControllerUUID != s.controllerUUID {
		return errors.NotValidf("secret URI with controller UUID %q", uri.ControllerUUID)
	}
	if arg.RotatePolicy == nil && arg.Description == nil && arg.ExpireTime == nil &&
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Data) == 0 {
		return errors.New("at least one attribute to update must be specified")
	}
	uri.ControllerUUID = s.controllerUUID
	if !s.canManage(uri, s.authTag) {
		return apiservererrors.ErrPerm
	}
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(0, nil); err != nil {
		return errors.Trace(err)
	}
	md, err := s.secretsService.GetSecret(ctx, uri)
	if err != nil {
		return errors.Trace(err)
	}
	var nextRotateTime *time.Time
	if !md.RotatePolicy.WillRotate() && arg.RotatePolicy.WillRotate() {
		nextRotateTime = arg.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	_, err = s.secretsService.UpdateSecret(ctx, uri, fromUpsertParams(arg.UpsertSecretArg, token, nextRotateTime))
	return errors.Trace(err)
}

// RemoveSecrets removes the specified secrets.
func (s *SecretsManagerAPI) RemoveSecrets(args params.SecretURIArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		err := s.removeSecret(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) removeSecret(ctx context.Context, arg params.SecretURIArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if uri.ControllerUUID != "" && uri.ControllerUUID != s.controllerUUID {
		return errors.NotValidf("secret URI with controller UUID %q", uri.ControllerUUID)
	}
	uri.ControllerUUID = s.controllerUUID
	if !s.canManage(uri, s.authTag) {
		return apiservererrors.ErrPerm
	}
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(0, nil); err != nil {
		return errors.Trace(err)
	}
	return s.secretsService.DeleteSecret(ctx, uri)
}

// GetLatestSecretsRevisionInfo returns the latest secret revisions for the specified secrets.
func (s *SecretsManagerAPI) GetLatestSecretsRevisionInfo(args params.GetSecretConsumerInfoArgs) (params.SecretConsumerInfoResults, error) {
	result := params.SecretConsumerInfoResults{
		Results: make([]params.SecretConsumerInfoResult, len(args.URIs)),
	}
	consumerTag, err := names.ParseTag(args.ConsumerTag)
	if err != nil {
		return params.SecretConsumerInfoResults{}, errors.Trace(err)
	}
	for i, uri := range args.URIs {
		data, err := s.getSecretConsumerInfo(consumerTag, uri)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = params.SecretConsumerInfoResult{
			Revision: data.LatestRevision,
			Label:    data.Label,
		}
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretConsumerInfo(consumerTag names.Tag, uriStr string) (*coresecrets.SecretConsumerMetadata, error) {
	uri, err := coresecrets.ParseURI(uriStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if uri.ControllerUUID == "" {
		uri.ControllerUUID = s.controllerUUID
	}
	if !s.canRead(uri, consumerTag) {
		return nil, apiservererrors.ErrPerm
	}
	return s.secretsConsumer.GetSecretConsumer(uri, consumerTag.String())
}

// GetSecretMetadata returns metadata for the caller's secrets.
func (s *SecretsManagerAPI) GetSecretMetadata() (params.ListSecretResults, error) {
	var result params.ListSecretResults
	ctx := context.Background()
	owner := names.NewApplicationTag(authTagApp(s.authTag)).String()
	secrets, _, err := s.secretsService.ListSecrets(ctx, secrets.Filter{
		OwnerTag: &owner,
	})
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ListSecretResult, len(secrets))
	for i, md := range secrets {
		result.Results[i] = params.ListSecretResult{
			URI:              md.URI.ShortString(),
			Version:          md.Version,
			RotatePolicy:     md.RotatePolicy.String(),
			NextRotateTime:   md.NextRotateTime,
			Description:      md.Description,
			Label:            md.Label,
			LatestRevision:   md.LatestRevision,
			LatestExpireTime: md.LatestExpireTime,
			CreateTime:       md.CreateTime,
			UpdateTime:       md.UpdateTime,
		}
	}
	return result, nil
}

// GetSecretValues returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretValues(args params.GetSecretValueArgs) (params.SecretValueResults, error) {
	result := params.SecretValueResults{
		Results: make([]params.SecretValueResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		data, err := s.getSecretValue(ctx, arg)
		result.Results[i].Data = data
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretValue(ctx context.Context, arg params.GetSecretValueArg) (coresecrets.SecretData, error) {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if uri.ControllerUUID == "" {
		uri.ControllerUUID = s.controllerUUID
	}
	if !s.canRead(uri, s.authTag) {
		return nil, apiservererrors.ErrPerm
	}
	consumer, err := s.secretsConsumer.GetSecretConsumer(uri, s.authTag.String())
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	update := arg.Update || err != nil
	peek := arg.Peek
	if update || peek {
		md, err := s.secretsService.GetSecret(ctx, uri)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if consumer == nil {
			consumer = &coresecrets.SecretConsumerMetadata{
				LatestRevision: md.LatestRevision,
			}
		}
		consumer.CurrentRevision = md.LatestRevision
		if arg.Label != "" {
			consumer.Label = arg.Label
		}

		if update {
			err := s.secretsConsumer.SaveSecretConsumer(uri, s.authTag.String(), consumer)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	val, err := s.secretsService.GetSecretValue(ctx, uri, consumer.CurrentRevision)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return val.EncodedValues(), nil
}

// WatchSecretsChanges sets up a watcher to notify of changes to secret revisions for the specified consumers.
func (s *SecretsManagerAPI) WatchSecretsChanges(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []string, error) {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		if !s.isSameApplication(tag) {
			return "", nil, apiservererrors.ErrPerm
		}
		w := s.secretsConsumer.WatchConsumedSecretsChanges(arg.Tag)
		if secretChanges, ok := <-w.Changes(); ok {
			changes := make([]string, len(secretChanges))
			copy(changes, secretChanges)
			return s.resources.Register(w), changes, nil
		}
		return "", nil, watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.StringsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.StringsWatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchSecretsRotationChanges sets up a watcher to notify of changes to secret rotation config.
func (s *SecretsManagerAPI) WatchSecretsRotationChanges(args params.Entities) (params.SecretTriggerWatchResults, error) {
	results := params.SecretTriggerWatchResults{
		Results: make([]params.SecretTriggerWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []params.SecretTriggerChange, error) {
		ownerTag, err := names.ParseTag(arg.Tag)
		if err != nil || authTagApp(s.authTag) != ownerTag.Id() {
			return "", nil, apiservererrors.ErrPerm
		}
		w := s.secretsRotation.WatchSecretsRotationChanges(ownerTag.String())
		if secretChanges, ok := <-w.Changes(); ok {
			changes := make([]params.SecretTriggerChange, len(secretChanges))
			for i, c := range secretChanges {
				changes[i] = params.SecretTriggerChange{
					URI:             c.URI.String(),
					NextTriggerTime: c.NextTriggerTime,
				}
			}
			return s.resources.Register(w), changes, nil
		}
		return "", nil, watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.SecretTriggerWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.WatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// SecretsRotated records when secrets were last rotated.
func (s *SecretsManagerAPI) SecretsRotated(args params.SecretRotatedArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	ctx := context.Background()
	one := func(arg params.SecretRotatedArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		uri.ControllerUUID = s.controllerUUID
		md, err := s.secretsService.GetSecret(ctx, uri)
		if err != nil {
			return errors.Trace(err)
		}
		owner, err := names.ParseTag(md.OwnerTag)
		if err != nil {
			return errors.Trace(err)
		}
		if authTagApp(s.authTag) != owner.Id() {
			return apiservererrors.ErrPerm
		}
		if !md.RotatePolicy.WillRotate() {
			logger.Debugf("secret %q was rotated but now is set to not rotate")
			return nil
		}
		lastRotateTime := md.NextRotateTime
		if lastRotateTime == nil {
			now := s.clock.Now()
			lastRotateTime = &now
		}
		var nextRotateTime time.Time
		logger.Debugf("secret %q was rotated: rev was %d, now %d", uri.ShortString(), arg.OriginalRevision, md.LatestRevision)
		if arg.Skip || md.LatestRevision > arg.OriginalRevision {
			nextRotateTime = *md.RotatePolicy.NextRotateTime(*lastRotateTime)
		} else {
			nextRotateTime = lastRotateTime.Add(coresecrets.RotateRetryDelay)
		}
		logger.Debugf("secret %q next rotate time is now: %s", uri.ShortString(), nextRotateTime.UTC().Format(time.RFC3339))
		return s.secretsRotation.SecretRotated(uri, nextRotateTime)
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}

type grantRevokeFunc func(*coresecrets.URI, state.SecretAccessParams) error

// SecretsGrant grants access to a secret for the specified subjects.
func (s *SecretsManagerAPI) SecretsGrant(args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(args, s.secretsConsumer.GrantSecretAccess)
}

// SecretsRevoke revokes access to a secret for the specified subjects.
func (s *SecretsManagerAPI) SecretsRevoke(args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(args, s.secretsConsumer.RevokeSecretAccess)
}

func (s *SecretsManagerAPI) secretsGrantRevoke(args params.GrantRevokeSecretArgs, op grantRevokeFunc) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(0, nil); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	one := func(arg params.GrantRevokeSecretArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		uri.ControllerUUID = s.controllerUUID
		if !s.canManage(uri, s.authTag) {
			return apiservererrors.ErrPerm
		}
		var scopeTag names.Tag
		if arg.ScopeTag != "" {
			var err error
			scopeTag, err = names.ParseTag(arg.ScopeTag)
			if err != nil {
				return errors.Trace(err)
			}
		}
		role := coresecrets.SecretRole(arg.Role)
		if role != "" && !role.IsValid() {
			return errors.NotValidf("secret role %q", arg.Role)
		}
		for _, tagStr := range arg.SubjectTags {
			subjectTag, err := names.ParseTag(tagStr)
			if err != nil {
				return errors.Trace(err)
			}
			if err := op(uri, state.SecretAccessParams{
				LeaderToken: token,
				Scope:       scopeTag,
				Subject:     subjectTag,
				Role:        role,
			}); err != nil {
				return errors.Annotatef(err, "cannot change access to %q for %q", uri, tagStr)
			}
		}
		return nil
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}
