// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
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
	modelUUID string

	leadershipChecker leadership.Checker
	secretsBackend    SecretsBackend
	resources         facade.Resources
	secretsRotation   SecretsRotation
	secretsConsumer   SecretsConsumer
	authTag           names.Tag
	clock             clock.Clock

	storeConfigGetter commonsecrets.StoreConfigGetter
}

// GetSecretStoreConfig gets the config needed to create a client to the model's secret store.
func (s *SecretsManagerAPI) GetSecretStoreConfig() (params.SecretStoreConfig, error) {
	cfg, err := s.storeConfigGetter()
	if err != nil {
		return params.SecretStoreConfig{}, errors.Trace(err)
	}
	result := params.SecretStoreConfig{
		StoreType: cfg.StoreType,
		Params:    cfg.Params,
	}
	return result, nil
}

// CreateSecretURIs creates new secret URIs.
func (s *SecretsManagerAPI) CreateSecretURIs(arg params.CreateSecretURIsArg) (params.StringResults, error) {
	if arg.Count <= 0 {
		return params.StringResults{}, errors.NotValidf("secret URi count %d", arg.Count)
	}
	result := params.StringResults{
		Results: make([]params.StringResult, arg.Count),
	}
	for i := 0; i < arg.Count; i++ {
		result.Results[i] = params.StringResult{Result: coresecrets.NewURI().String()}
	}
	return result, nil
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		ID, err := s.createSecret(arg)
		result.Results[i].Result = ID
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) createSecret(arg params.CreateSecretArg) (string, error) {
	if len(arg.Content.Data) == 0 && arg.Content.ProviderId == nil {
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
	var uri *coresecrets.URI
	if arg.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.URI)
		if err != nil {
			return "", errors.Trace(err)
		}
	} else {
		uri = coresecrets.NewURI()
	}
	var nextRotateTime *time.Time
	if arg.RotatePolicy.WillRotate() {
		nextRotateTime = arg.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	md, err := s.secretsBackend.CreateSecret(uri, state.CreateSecretParams{
		Version:            secrets.Version,
		Owner:              arg.OwnerTag,
		UpdateSecretParams: fromUpsertParams(arg.UpsertSecretArg, token, nextRotateTime),
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
		if err2 := s.secretsBackend.DeleteSecret(uri); err2 != nil {
			logger.Warningf("cleaning up secret %q", uri)
		}
		return "", errors.Annotate(err, "granting secret owner permission to manage the secret")
	}
	return md.URI.String(), nil
}

func fromUpsertParams(p params.UpsertSecretArg, token leadership.Token, nextRotateTime *time.Time) state.UpdateSecretParams {
	return state.UpdateSecretParams{
		LeaderToken:    token,
		RotatePolicy:   p.RotatePolicy,
		NextRotateTime: nextRotateTime,
		ExpireTime:     p.ExpireTime,
		Description:    p.Description,
		Label:          p.Label,
		Params:         p.Params,
		Data:           p.Content.Data,
		ProviderId:     p.Content.ProviderId,
	}
}

// UpdateSecrets updates the specified secrets.
func (s *SecretsManagerAPI) UpdateSecrets(args params.UpdateSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := s.updateSecret(arg)
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) updateSecret(arg params.UpdateSecretArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if arg.RotatePolicy == nil && arg.Description == nil && arg.ExpireTime == nil &&
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Content.Data) == 0 && arg.Content.ProviderId == nil {
		return errors.New("at least one attribute to update must be specified")
	}
	if !s.canManage(uri, s.authTag) {
		return apiservererrors.ErrPerm
	}
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(0, nil); err != nil {
		return errors.Trace(err)
	}
	md, err := s.secretsBackend.GetSecret(uri)
	if err != nil {
		return errors.Trace(err)
	}
	var nextRotateTime *time.Time
	if !md.RotatePolicy.WillRotate() && arg.RotatePolicy.WillRotate() {
		nextRotateTime = arg.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	_, err = s.secretsBackend.UpdateSecret(uri, fromUpsertParams(arg.UpsertSecretArg, token, nextRotateTime))
	return errors.Trace(err)
}

// RemoveSecrets removes the specified secrets.
func (s *SecretsManagerAPI) RemoveSecrets(args params.SecretURIArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := s.removeSecret(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) removeSecret(arg params.SecretURIArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if !s.canManage(uri, s.authTag) {
		return apiservererrors.ErrPerm
	}
	appName := authTagApp(s.authTag)
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	if err := token.Check(0, nil); err != nil {
		return errors.Trace(err)
	}
	return s.secretsBackend.DeleteSecret(uri)
}

// GetConsumerSecretsRevisionInfo returns the latest secret revisions for the specified secrets.
func (s *SecretsManagerAPI) GetConsumerSecretsRevisionInfo(args params.GetSecretConsumerInfoArgs) (params.SecretConsumerInfoResults, error) {
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
	if !s.canRead(uri, consumerTag) {
		return nil, apiservererrors.ErrPerm
	}
	return s.secretsConsumer.GetSecretConsumer(uri, consumerTag.String())
}

// GetSecretMetadata returns metadata for the caller's secrets.
func (s *SecretsManagerAPI) GetSecretMetadata() (params.ListSecretResults, error) {
	var result params.ListSecretResults
	owner := names.NewApplicationTag(authTagApp(s.authTag)).String()
	secrets, err := s.secretsBackend.ListSecrets(state.SecretsFilter{
		OwnerTag: &owner,
	})
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ListSecretResult, len(secrets))
	for i, md := range secrets {
		result.Results[i] = params.ListSecretResult{
			URI:              md.URI.String(),
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
		revs, err := s.secretsBackend.ListSecretRevisions(md.URI)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		for _, r := range revs {
			result.Results[i].Revisions = append(result.Results[i].Revisions, params.SecretRevision{
				Revision:   r.Revision,
				ProviderId: r.ProviderId,
			})
		}
	}
	return result, nil
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretContentInfo(args params.GetSecretContentArgs) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		content, err := s.getSecretContent(arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		contentParams := params.SecretContentParams{
			ProviderId: content.ProviderId,
		}
		if content.SecretValue != nil {
			contentParams.Data = content.SecretValue.EncodedValues()
		}
		result.Results[i].Content = contentParams
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretContent(arg params.GetSecretContentArg) (*secrets.ContentParams, error) {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return nil, errors.Trace(err)
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
		md, err := s.secretsBackend.GetSecret(uri)
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

	val, providerId, err := s.secretsBackend.GetSecretValue(uri, consumer.CurrentRevision)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secrets.ContentParams{SecretValue: val, ProviderId: providerId}, nil
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
	one := func(arg params.SecretRotatedArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		md, err := s.secretsBackend.GetSecret(uri)
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
		logger.Debugf("secret %q was rotated: rev was %d, now %d", uri.String(), arg.OriginalRevision, md.LatestRevision)
		if arg.Skip || md.LatestRevision > arg.OriginalRevision {
			nextRotateTime = *md.RotatePolicy.NextRotateTime(*lastRotateTime)
		} else {
			nextRotateTime = lastRotateTime.Add(coresecrets.RotateRetryDelay)
		}
		logger.Debugf("secret %q next rotate time is now: %s", uri.String(), nextRotateTime.UTC().Format(time.RFC3339))
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
