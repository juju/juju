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
	secretsprovider "github.com/juju/juju/secrets/provider"
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
	secretsTriggers   SecretTriggers
	secretsConsumer   SecretsConsumer
	authTag           names.Tag
	clock             clock.Clock

	storeConfigGetter commonsecrets.BackendConfigGetter
	providerGetter    commonsecrets.ProviderInfoGetter
}

// GetSecretStoreConfig gets the config needed to create a client to the model's secret store.
func (s *SecretsManagerAPI) GetSecretStoreConfig() (params.SecretStoreConfig, error) {
	cfg, err := s.storeConfigGetter()
	if err != nil {
		return params.SecretStoreConfig{}, errors.Trace(err)
	}
	result := params.SecretStoreConfig{
		StoreType: cfg.BackendType,
		Params:    cfg.Config,
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
	if len(arg.Content.Data) == 0 && arg.Content.BackendId == nil {
		return "", errors.NotValidf("empty secret value")
	}
	// A unit can only create secrets owned by its app.
	secretOwner, err := names.ParseTag(arg.OwnerTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	token, err := s.ownerToken(secretOwner)
	if err != nil {
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
		Owner:              secretOwner,
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
		if _, err2 := s.secretsBackend.DeleteSecret(uri); err2 != nil {
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
		BackendId:      p.Content.BackendId,
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
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Content.Data) == 0 && arg.Content.BackendId == nil {
		return errors.New("at least one attribute to update must be specified")
	}
	token, err := s.canManage(uri)
	if err != nil {
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
func (s *SecretsManagerAPI) RemoveSecrets(args params.DeleteSecretArgs) (params.ErrorResults, error) {
	type deleteInfo struct {
		uri       *coresecrets.URI
		revisions []int
	}
	toDelete := make([]deleteInfo, len(args.Args))
	for i, arg := range args.Args {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}
		toDelete[i] = deleteInfo{uri: uri, revisions: arg.Revisions}
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	removedRevisions := secretsprovider.SecretRevisions{}
	for i, d := range toDelete {
		removed, err := s.removeSecret(d.uri, d.revisions...)
		result.Results[i].Error = apiservererrors.ServerError(err)
		if err == nil && removed {
			removedRevisions.Add(d.uri, d.revisions...)
		}
	}
	if len(removedRevisions) == 0 {
		return result, nil
	}

	provider, model, err := s.providerGetter()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	// TODO: include unitTag in params.DeleteSecretArgs for operator uniters?
	// This should be resolved once lp:1991213 and lp:1991854 are fixed.
	if err := provider.CleanupSecrets(model, s.authTag, removedRevisions); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) removeSecret(uri *coresecrets.URI, revisions ...int) (bool, error) {
	_, err := s.canManage(uri)
	if err != nil {
		return false, errors.Trace(err)
	}
	return s.secretsBackend.DeleteSecret(uri, revisions...)
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
	return s.secretsConsumer.GetSecretConsumer(uri, consumerTag)
}

// GetSecretMetadata returns metadata for the caller's secrets.
func (s *SecretsManagerAPI) GetSecretMetadata() (params.ListSecretResults, error) {
	var result params.ListSecretResults
	filter := state.SecretsFilter{
		OwnerTags: []names.Tag{s.authTag},
	}
	// Unit leaders can also get metadata for secrets owned by the app.
	// TODO(wallyworld) - temp fix for old podspec charms
	isLeader, err := s.isLeaderUnit()
	if err != nil {
		return result, errors.Trace(err)
	}
	if isLeader {
		appOwner := names.NewApplicationTag(authTagApp(s.authTag))
		filter.OwnerTags = append(filter.OwnerTags, appOwner)
	}

	secrets, err := s.secretsBackend.ListSecrets(filter)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ListSecretResult, len(secrets))
	for i, md := range secrets {
		result.Results[i] = params.ListSecretResult{
			URI:              md.URI.String(),
			Version:          md.Version,
			OwnerTag:         md.OwnerTag,
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
				Revision:  r.Revision,
				BackendId: r.BackendId,
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
			BackendId: content.BackendId,
		}
		if content.SecretValue != nil {
			contentParams.Data = content.SecretValue.EncodedValues()
		}
		result.Results[i].Content = contentParams
	}
	return result, nil
}

func (s *SecretsManagerAPI) isLeaderUnit() (bool, error) {
	if s.authTag.Kind() != names.UnitTagKind {
		return false, nil
	}
	_, err := s.leadershipToken()
	if err != nil && !leadership.IsNotLeaderError(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}

func (s *SecretsManagerAPI) handleAppOwnedSecretForUnits(md *coresecrets.SecretMetadata) (*coresecrets.SecretMetadata, error) {
	// If the secret is owned by the app, and the caller is a peer unit, we create a fake consumer doc for triggering events to notify the uniters.
	// The peer units should get the secret using owner label but should not set a consumer label.
	consumer, err := s.secretsConsumer.GetSecretConsumer(md.URI, s.authTag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	if consumer == nil {
		// Create a fake consumer doc for triggering secret-changed event for uniter.
		consumer = &coresecrets.SecretConsumerMetadata{}
	}
	logger.Debugf("saving consumer doc for application owned secret %q for peer units %q", md.URI, s.authTag)
	if err := s.secretsConsumer.SaveSecretConsumer(md.URI, s.authTag, consumer); err != nil {
		return nil, errors.Trace(err)
	}
	return md, nil
}

func (s *SecretsManagerAPI) getAppOwnedOrUnitOwnedSecretMetadata(uri *coresecrets.URI, label string) (md *coresecrets.SecretMetadata, err error) {
	notFoundErr := errors.NotFoundf("secret %q", uri)
	if label != "" {
		notFoundErr = errors.NotFoundf("secret with label %q", label)
	}
	defer func() {
		if md == nil || md.OwnerTag == s.authTag.String() {
			// Either errored out or found a secret owned by the caller.
			return
		}
		md, err = s.handleAppOwnedSecretForUnits(md)
	}()

	filter := state.SecretsFilter{
		OwnerTags: []names.Tag{s.authTag},
	}
	if s.authTag.Kind() == names.UnitTagKind {
		// Units can access application owned secrets.
		appOwner := names.NewApplicationTag(authTagApp(s.authTag))
		filter.OwnerTags = append(filter.OwnerTags, appOwner)
	}
	mds, err := s.secretsBackend.ListSecrets(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, md := range mds {
		if uri != nil && md.URI.ID == uri.ID {
			return md, nil
		}
		if label != "" && md.Label == label {
			return md, nil
		}
	}
	return nil, notFoundErr
}

func (s *SecretsManagerAPI) getSecretContent(arg params.GetSecretContentArg) (*secrets.ContentParams, error) {
	// Only the owner can access secrets via the secret metadata label added by the owner.
	// (Note: the leader unit is not the owner of the application secrets).
	// Consumers get to use their own label.
	// Both owners and consumers can also just use the secret URI.

	if arg.URI == "" && arg.Label == "" {
		return nil, errors.NewNotValid(nil, "both uri and label are empty")
	}

	getSecretValue := func(uri *coresecrets.URI, revision int) (*secrets.ContentParams, error) {
		if !s.canRead(uri, s.authTag) {
			return nil, apiservererrors.ErrPerm
		}

		val, backendId, err := s.secretsBackend.GetSecretValue(uri, revision)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &secrets.ContentParams{SecretValue: val, BackendId: backendId}, nil
	}

	var uri *coresecrets.URI
	var err error

	if arg.URI != "" {
		uri, err = coresecrets.ParseURI(arg.URI)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	// Owner units should always have the UIR because we resolved the label to URI on uniter side already.
	md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(uri, arg.Label)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if md != nil {
		// 1. secrets can be accesed by the owner;
		// 2. application owned secrets can be accessed by all the units of the application using owner label or URI.
		return getSecretValue(md.URI, md.LatestRevision)
	}

	// arg.Label is the consumer label for consumers.
	possibleUpdateLabel := arg.Label != "" && uri != nil

	if uri == nil {
		var err error
		uri, err = s.secretsConsumer.GetURIByConsumerLabel(arg.Label, s.authTag)
		if errors.Is(err, errors.NotFound) {
			return nil, errors.NotFoundf("consumer label %q", arg.Label)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	consumer, err := s.secretsConsumer.GetSecretConsumer(uri, s.authTag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	refresh := arg.Refresh ||
		err != nil // Not found, so need to create one.
	peek := arg.Peek

	// Use the latest revision as the current one if --refresh or --peek.
	if refresh || peek {
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
	}
	if refresh || possibleUpdateLabel {
		if arg.Label != "" {
			consumer.Label = arg.Label
		}
		if err := s.secretsConsumer.SaveSecretConsumer(uri, s.authTag, consumer); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return getSecretValue(uri, consumer.CurrentRevision)
}

// WatchConsumedSecretsChanges sets up a watcher to notify of changes to secret revisions for the specified consumers.
func (s *SecretsManagerAPI) WatchConsumedSecretsChanges(args params.Entities) (params.StringsWatchResults, error) {
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
		w, err := s.secretsConsumer.WatchConsumedSecretsChanges(tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		if changes, ok := <-w.Changes(); ok {
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

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *SecretsManagerAPI) WatchObsolete(args params.Entities) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	owners := make([]names.Tag, len(args.Entities))
	for i, arg := range args.Entities {
		ownerTag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return result, errors.Trace(err)
		}
		if !s.isSameApplication(ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		// Only unit leaders can watch application secrets.
		// TODO(wallyworld) - temp fix for old podspec charms
		if ownerTag.Kind() == names.ApplicationTagKind && s.authTag.Kind() != names.ApplicationTagKind {
			_, err := s.leadershipToken()
			if err != nil {
				return result, errors.Trace(err)
			}
		}
		owners[i] = ownerTag
	}
	w, err := s.secretsBackend.WatchObsolete(owners)
	if err != nil {
		return result, errors.Trace(err)
	}
	if changes, ok := <-w.Changes(); ok {
		result.StringsWatcherId = s.resources.Register(w)
		result.Changes = changes
	} else {
		err = watcher.EnsureErr(w)
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// WatchSecretsRotationChanges sets up a watcher to notify of changes to secret rotation config.
func (s *SecretsManagerAPI) WatchSecretsRotationChanges(args params.Entities) (params.SecretTriggerWatchResult, error) {
	result := params.SecretTriggerWatchResult{}
	owners := make([]names.Tag, len(args.Entities))
	for i, arg := range args.Entities {
		ownerTag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return result, errors.Trace(err)
		}
		if !s.isSameApplication(ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		// Only unit leaders can watch application secrets.
		// TODO(wallyworld) - temp fix for old podspec charms
		if ownerTag.Kind() == names.ApplicationTagKind && s.authTag.Kind() != names.ApplicationTagKind {
			_, err := s.leadershipToken()
			if err != nil {
				return result, errors.Trace(err)
			}
		}
		owners[i] = ownerTag
	}
	w, err := s.secretsTriggers.WatchSecretsRotationChanges(owners)
	if err != nil {
		return result, errors.Trace(err)
	}
	if secretChanges, ok := <-w.Changes(); ok {
		changes := make([]params.SecretTriggerChange, len(secretChanges))
		for i, c := range secretChanges {
			changes[i] = params.SecretTriggerChange{
				URI:             c.URI.String(),
				NextTriggerTime: c.NextTriggerTime,
			}
		}
		result.WatcherId = s.resources.Register(w)
		result.Changes = changes
	} else {
		err = watcher.EnsureErr(w)
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
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
		nextRotateTime := *md.RotatePolicy.NextRotateTime(*lastRotateTime)
		logger.Debugf("secret %q was rotated: rev was %d, now %d", uri.String(), arg.OriginalRevision, md.LatestRevision)
		// If the secret will expire before it is due to be next rotated, rotate sooner to allow
		// the charm a chance to update it before it expires.
		willExpire := md.LatestExpireTime != nil && md.LatestExpireTime.Before(nextRotateTime)
		forcedRotateTime := lastRotateTime.Add(coresecrets.RotateRetryDelay)
		if willExpire {
			logger.Warningf("secret %q rev %d will expire before next scheduled rotation", uri.String(), md.LatestRevision)
		}
		if willExpire && forcedRotateTime.Before(*md.LatestExpireTime) || !arg.Skip && md.LatestRevision == arg.OriginalRevision {
			nextRotateTime = forcedRotateTime
		}
		logger.Debugf("secret %q next rotate time is now: %s", uri.String(), nextRotateTime.UTC().Format(time.RFC3339))
		return s.secretsTriggers.SecretRotated(uri, nextRotateTime)
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}

// WatchSecretRevisionsExpiryChanges sets up a watcher to notify of changes to secret revision expiry config.
func (s *SecretsManagerAPI) WatchSecretRevisionsExpiryChanges(args params.Entities) (params.SecretTriggerWatchResult, error) {
	result := params.SecretTriggerWatchResult{}
	owners := make([]names.Tag, len(args.Entities))
	for i, arg := range args.Entities {
		ownerTag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return result, errors.Trace(err)
		}
		if !s.isSameApplication(ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		// Only unit leaders can watch application secrets.
		// TODO(wallyworld) - temp fix for old podspec charms
		if ownerTag.Kind() == names.ApplicationTagKind && s.authTag.Kind() != names.ApplicationTagKind {
			_, err := s.leadershipToken()
			if err != nil {
				return result, errors.Trace(err)
			}
		}
		owners[i] = ownerTag
	}
	w, err := s.secretsTriggers.WatchSecretRevisionsExpiryChanges(owners)
	if err != nil {
		return result, errors.Trace(err)
	}
	if secretChanges, ok := <-w.Changes(); ok {
		changes := make([]params.SecretTriggerChange, len(secretChanges))
		for i, c := range secretChanges {
			changes[i] = params.SecretTriggerChange{
				URI:             c.URI.String(),
				Revision:        c.Revision,
				NextTriggerTime: c.NextTriggerTime,
			}
		}
		result.WatcherId = s.resources.Register(w)
		result.Changes = changes
	} else {
		err = watcher.EnsureErr(w)
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
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
	one := func(arg params.GrantRevokeSecretArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
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
		token, err := s.canManage(uri)
		if err != nil {
			return errors.Trace(err)
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
