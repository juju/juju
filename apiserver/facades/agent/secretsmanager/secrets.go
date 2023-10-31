// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLoggerWithLabels("juju.apiserver.secretsmanager", corelogger.SECRETS)

// CrossModelSecretsClient gets secret content from a cross model controller.
type CrossModelSecretsClient interface {
	GetRemoteSecretContentInfo(uri *coresecrets.URI, revision int, refresh, peek bool, appToken string, unitId int, macs macaroon.Slice) (*secrets.ContentParams, *secretsprovider.ModelBackendConfig, int, bool, error)
	GetSecretAccessScope(uri *coresecrets.URI, appToken string, unitId int) (string, error)
}

// SecretsManagerAPI is the implementation for the SecretsManager facade.
type SecretsManagerAPI struct {
	authorizer        facade.Authorizer
	leadershipChecker leadership.Checker
	secretsState      SecretsState
	resources         facade.Resources
	secretsTriggers   SecretTriggers
	secretsConsumer   SecretsConsumer
	authTag           names.Tag
	clock             clock.Clock
	modelUUID         string

	backendConfigGetter commonsecrets.BackendConfigGetter
	adminConfigGetter   commonsecrets.BackendAdminConfigGetter
	drainConfigGetter   commonsecrets.BackendDrainConfigGetter
	remoteClientGetter  func(uri *coresecrets.URI) (CrossModelSecretsClient, error)

	crossModelState CrossModelState
}

// SecretsManagerAPIV1 the secrets manager facade v1.
// TODO - drop when we no longer support juju 3.1.0
type SecretsManagerAPIV1 struct {
	*SecretsManagerAPI
}

func (s *SecretsManagerAPI) canRead(uri *coresecrets.URI, entity names.Tag) bool {
	return commonsecrets.CanRead(s.secretsConsumer, s.authTag, uri, entity)
}

func (s *SecretsManagerAPI) canManage(uri *coresecrets.URI) (leadership.Token, error) {
	return commonsecrets.CanManage(s.secretsConsumer, s.leadershipChecker, s.authTag, uri)
}

// GetSecretStoreConfig is for 3.0.x agents.
// TODO - drop when we no longer support juju 3.0.x
func (s *SecretsManagerAPIV1) GetSecretStoreConfig() (params.SecretBackendConfig, error) {
	cfgInfo, err := s.GetSecretBackendConfig()
	if err != nil {
		return params.SecretBackendConfig{}, errors.Trace(err)
	}
	return cfgInfo.Configs[cfgInfo.ActiveID], nil
}

// GetSecretBackendConfig gets the config needed to create a client to secret backends.
// TODO - drop when we no longer support juju 3.1.x
func (s *SecretsManagerAPIV1) GetSecretBackendConfig() (params.SecretBackendConfigResultsV1, error) {
	cfgInfo, err := s.backendConfigGetter(nil, true)
	if err != nil {
		return params.SecretBackendConfigResultsV1{}, errors.Trace(err)
	}
	result := params.SecretBackendConfigResultsV1{
		ActiveID: cfgInfo.ActiveID,
		Configs:  make(map[string]params.SecretBackendConfig),
	}
	for id, cfg := range cfgInfo.Configs {
		result.ControllerUUID = cfg.ControllerUUID
		result.ModelUUID = cfg.ModelUUID
		result.ModelName = cfg.ModelName
		result.Configs[id] = params.SecretBackendConfig{
			BackendType: cfg.BackendType,
			Params:      cfg.Config,
		}
	}
	return result, nil
}

// GetSecretBackendConfigs isn't on the V1 API.
func (*SecretsManagerAPIV1) GetSecretBackendConfigs(_ struct{}) {}

// GetSecretBackendConfigs gets the config needed to create a client to secret backends.
func (s *SecretsManagerAPI) GetSecretBackendConfigs(arg params.SecretBackendArgs) (params.SecretBackendConfigResults, error) {
	if arg.ForDrain {
		return s.getBackendConfigForDrain(arg)
	}
	results := params.SecretBackendConfigResults{
		Results: make(map[string]params.SecretBackendConfigResult, len(arg.BackendIDs)),
	}
	result, activeID, err := s.getSecretBackendConfig(arg.BackendIDs)
	if err != nil {
		return results, errors.Trace(err)
	}
	results.ActiveID = activeID
	results.Results = result
	return results, nil
}

// GetBackendConfigForDrain fetches the config needed to make a secret backend client for the drain worker.
func (s *SecretsManagerAPI) getBackendConfigForDrain(arg params.SecretBackendArgs) (params.SecretBackendConfigResults, error) {
	if len(arg.BackendIDs) > 1 {
		return params.SecretBackendConfigResults{}, errors.Errorf("Maximumly only one backend ID can be specified for drain")
	}
	var backendID string
	if len(arg.BackendIDs) == 1 {
		backendID = arg.BackendIDs[0]
	}
	results := params.SecretBackendConfigResults{
		Results: make(map[string]params.SecretBackendConfigResult, 1),
	}
	cfgInfo, err := s.drainConfigGetter(backendID)
	if err != nil {
		return results, errors.Trace(err)
	}
	if len(cfgInfo.Configs) == 0 {
		return results, errors.NotFoundf("no secret backends available")
	}
	results.ActiveID = cfgInfo.ActiveID
	for id, cfg := range cfgInfo.Configs {
		results.Results[id] = params.SecretBackendConfigResult{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			Draining:       true,
			Config: params.SecretBackendConfig{
				BackendType: cfg.BackendType,
				Params:      cfg.Config,
			},
		}
	}
	return results, nil
}

// GetSecretBackendConfig gets the config needed to create a client to secret backends.
func (s *SecretsManagerAPI) getSecretBackendConfig(backendIDs []string) (map[string]params.SecretBackendConfigResult, string, error) {
	cfgInfo, err := s.backendConfigGetter(backendIDs, false)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	result := make(map[string]params.SecretBackendConfigResult)
	wanted := set.NewStrings(backendIDs...)
	for id, cfg := range cfgInfo.Configs {
		if len(wanted) > 0 {
			if !wanted.Contains(id) {
				continue
			}
		} else if id != cfgInfo.ActiveID {
			continue
		}
		result[id] = params.SecretBackendConfigResult{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			Draining:       id != cfgInfo.ActiveID,
			Config: params.SecretBackendConfig{
				BackendType: cfg.BackendType,
				Params:      cfg.Config,
			},
		}
	}
	return result, cfgInfo.ActiveID, nil
}

func (s *SecretsManagerAPI) getBackend(backendID string) (*secretsprovider.ModelBackendConfig, bool, error) {
	cfgInfo, err := s.backendConfigGetter([]string{backendID}, false)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	cfg, ok := cfgInfo.Configs[backendID]
	if ok {
		return &secretsprovider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: secretsprovider.BackendConfig{
				BackendType: cfg.BackendType,
				Config:      cfg.Config,
			},
		}, backendID != cfgInfo.ActiveID, nil
	}
	return nil, false, errors.NotFoundf("secret backend %q", backendID)
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
		uri := coresecrets.NewURI().WithSource(s.modelUUID)
		result.Results[i] = params.StringResult{Result: uri.String()}
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
	if len(arg.Content.Data) == 0 && arg.Content.ValueRef == nil {
		return "", errors.NotValidf("empty secret value")
	}
	// A unit can only create secrets owned by its app.
	secretOwner, err := names.ParseTag(arg.OwnerTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	token, err := commonsecrets.OwnerToken(s.authTag, secretOwner, s.leadershipChecker)
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
	md, err := s.secretsState.CreateSecret(uri, state.CreateSecretParams{
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
		if _, err2 := s.secretsState.DeleteSecret(uri); err2 != nil {
			logger.Warningf("cleaning up secret %q", uri)
		}
		return "", errors.Annotate(err, "granting secret owner permission to manage the secret")
	}
	return md.URI.String(), nil
}

func fromUpsertParams(p params.UpsertSecretArg, token leadership.Token, nextRotateTime *time.Time) state.UpdateSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return state.UpdateSecretParams{
		LeaderToken:    token,
		RotatePolicy:   p.RotatePolicy,
		NextRotateTime: nextRotateTime,
		ExpireTime:     p.ExpireTime,
		Description:    p.Description,
		Label:          p.Label,
		Params:         p.Params,
		Data:           p.Content.Data,
		ValueRef:       valueRef,
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
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Content.Data) == 0 && arg.Content.ValueRef == nil {
		return errors.New("at least one attribute to update must be specified")
	}
	if _, err := s.secretsState.GetSecret(uri); err != nil {
		// Check if the uri exists or not.
		return errors.Trace(err)
	}

	token, err := s.canManage(uri)
	if err != nil {
		return errors.Trace(err)
	}
	md, err := s.secretsState.GetSecret(uri)
	if err != nil {
		return errors.Trace(err)
	}
	var nextRotateTime *time.Time
	if !md.RotatePolicy.WillRotate() && arg.RotatePolicy.WillRotate() {
		nextRotateTime = arg.RotatePolicy.NextRotateTime(s.clock.Now())
	}
	_, err = s.secretsState.UpdateSecret(uri, fromUpsertParams(arg.UpsertSecretArg, token, nextRotateTime))
	return errors.Trace(err)
}

// RemoveSecrets removes the specified secrets.
func (s *SecretsManagerAPI) RemoveSecrets(args params.DeleteSecretArgs) (params.ErrorResults, error) {
	return commonsecrets.RemoveSecretsForAgent(
		s.secretsState, s.adminConfigGetter, args,
		func(uri *coresecrets.URI) error {
			_, err := s.canManage(uri)
			return errors.Trace(err)
		},
	)
}

// GetConsumerSecretsRevisionInfo returns the latest secret revisions for the specified secrets.
// This facade method is used for remote watcher to get the latest secret revisions and labels for a secret changed hook.
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
	// We only check read permissions for local secrets.
	// For CMR secrets, the remote model manages the permissions.
	if uri.IsLocal(s.modelUUID) && !s.canRead(uri, consumerTag) {
		return nil, apiservererrors.ErrPerm
	}
	consumer, err := s.secretsConsumer.GetSecretConsumer(uri, consumerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if consumer.Label != "" {
		return consumer, nil
	}
	md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(uri, "", false, false)
	if errors.Is(err, errors.NotFound) {
		// The secret is owned by a different application.
		return consumer, nil
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get secret metadata for %q", uri)
	}
	// We allow units to access the application owned secrets using the application owner label,
	// so we copy the owner label to consumer metadata.
	consumer.Label = md.Label
	return consumer, nil
}

// GetSecretMetadata returns metadata for the caller's secrets.
func (s *SecretsManagerAPI) GetSecretMetadata() (params.ListSecretResults, error) {
	return commonsecrets.GetSecretMetadata(s.authTag, s.secretsState, s.leadershipChecker, nil)
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretContentInfo(args params.GetSecretContentArgs) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		content, backend, draining, err := s.getSecretContent(arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		contentParams := params.SecretContentParams{}
		if content.ValueRef != nil {
			contentParams.ValueRef = &params.SecretValueRef{
				BackendID:  content.ValueRef.BackendID,
				RevisionID: content.ValueRef.RevisionID,
			}
		}
		if content.SecretValue != nil {
			contentParams.Data = content.SecretValue.EncodedValues()
		}
		result.Results[i].Content = contentParams
		if backend != nil {
			result.Results[i].BackendConfig = &params.SecretBackendConfigResult{
				ControllerUUID: backend.ControllerUUID,
				ModelUUID:      backend.ModelUUID,
				ModelName:      backend.ModelName,
				Draining:       draining,
				Config: params.SecretBackendConfig{
					BackendType: backend.BackendType,
					Params:      backend.Config,
				},
			}
		}
	}
	return result, nil
}

func (s *SecretsManagerAPI) getRemoteSecretContent(uri *coresecrets.URI, refresh, peek bool, label string, updateLabel bool) (
	*secrets.ContentParams, *secretsprovider.ModelBackendConfig, bool, error,
) {
	extClient, err := s.remoteClientGetter(uri)
	if err != nil {
		return nil, nil, false, errors.Annotate(err, "creating remote secret client")
	}
	consumerApp := commonsecrets.AuthTagApp(s.authTag)
	token, err := s.crossModelState.GetToken(names.NewApplicationTag(consumerApp))
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote token for %q", consumerApp)
	}
	var unitId int
	if unitTag, ok := s.authTag.(names.UnitTag); ok {
		unitId = unitTag.Number()
	} else {
		return nil, nil, false, errors.NotSupportedf("getting cross model secret for consumer %q", s.authTag)
	}

	consumerInfo, err := s.secretsConsumer.GetSecretConsumer(uri, s.authTag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, nil, false, errors.Trace(err)
	}
	var wantRevision int
	if err == nil {
		wantRevision = consumerInfo.CurrentRevision
	} else {
		// Not found so need to create a new record and populate
		// with latest revision.
		refresh = true
		consumerInfo = &coresecrets.SecretConsumerMetadata{}
	}

	scopeToken, err := extClient.GetSecretAccessScope(uri, token, unitId)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, nil, false, apiservererrors.ErrPerm
		}
		return nil, nil, false, errors.Trace(err)
	}
	logger.Debugf("secret %q scope token for %v: %s", uri.String(), token, scopeToken)

	scopeEntity, err := s.crossModelState.GetRemoteEntity(scopeToken)
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote entity for %q", scopeToken)
	}
	logger.Debugf("secret %q scope for %v: %s", uri.String(), scopeToken, scopeEntity)

	mac, err := s.crossModelState.GetMacaroon(scopeEntity)
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote mac for %q", scopeEntity)
	}

	macs := macaroon.Slice{mac}
	content, backend, latestRevision, draining, err := extClient.GetRemoteSecretContentInfo(uri, wantRevision, refresh, peek, token, unitId, macs)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	if refresh || updateLabel {
		if refresh {
			consumerInfo.LatestRevision = latestRevision
			consumerInfo.CurrentRevision = latestRevision
		}
		if label != "" {
			consumerInfo.Label = label
		}
		if err := s.secretsConsumer.SaveSecretConsumer(uri, s.authTag, consumerInfo); err != nil {
			return nil, nil, false, errors.Trace(err)
		}
	}
	return content, backend, draining, nil
}

// GetSecretRevisionContentInfo returns the secret values for the specified secret revisions.
func (s *SecretsManagerAPI) GetSecretRevisionContentInfo(arg params.SecretRevisionArg) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(arg.Revisions)),
	}
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return params.SecretContentResults{}, errors.Trace(err)
	}
	if _, err = s.canManage(uri); err != nil {
		return params.SecretContentResults{}, errors.Trace(err)
	}
	for i, rev := range arg.Revisions {
		// TODO(wallworld) - if pendingDelete is true, mark the revision for deletion
		val, valueRef, err := s.secretsState.GetSecretValue(uri, rev)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		contentParams := params.SecretContentParams{}
		if valueRef != nil {
			contentParams.ValueRef = &params.SecretValueRef{
				BackendID:  valueRef.BackendID,
				RevisionID: valueRef.RevisionID,
			}
			backend, draining, err := s.getBackend(valueRef.BackendID)
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			result.Results[i].BackendConfig = &params.SecretBackendConfigResult{
				ControllerUUID: backend.ControllerUUID,
				ModelUUID:      backend.ModelUUID,
				ModelName:      backend.ModelName,
				Draining:       draining,
				Config: params.SecretBackendConfig{
					BackendType: backend.BackendType,
					Params:      backend.Config,
				},
			}
		}
		if val != nil {
			contentParams.Data = val.EncodedValues()
		}
		result.Results[i].Content = contentParams
	}
	return result, nil
}

// For the application owned secret, if the caller is a peer unit, we create a fake consumer doc for triggering events to notify the uniters.
// The peer units should get the secret using owner label but should not set a consumer label.
func (s *SecretsManagerAPI) ensureConsumerMetadataForAppOwnedSecretsForPeerUnits(md *coresecrets.SecretMetadata) (*coresecrets.SecretMetadata, error) {
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

func (s *SecretsManagerAPI) updateLabelForAppOwnedOrUnitOwnedSecret(uri *coresecrets.URI, label string, md *coresecrets.SecretMetadata) error {
	if uri == nil || label == "" {
		// We have done this check before, but it doesn't hurt to do it again.
		return nil
	}

	ownerTag, err := names.ParseTag(md.OwnerTag)
	if err != nil {
		return errors.Trace(err)
	}
	isLeaderUnit, err := commonsecrets.IsLeaderUnit(s.authTag, s.leadershipChecker)
	if err != nil {
		return errors.Trace(err)
	}
	if ownerTag == s.authTag || commonsecrets.IsSameApplication(ownerTag, s.authTag) && isLeaderUnit {
		// The secret is owned by the caller or the caller is the leader unit of the application owning the secret.
		token, err := commonsecrets.LeadershipToken(s.authTag, s.leadershipChecker)
		if err != nil {
			return errors.Trace(err)
		}
		// Update the label.
		_, err = s.secretsState.UpdateSecret(uri, state.UpdateSecretParams{
			LeaderToken: token,
			Label:       &label,
		})
		return errors.Trace(err)
	}
	return nil
}

func (s *SecretsManagerAPI) getAppOwnedOrUnitOwnedSecretMetadata(uri *coresecrets.URI, label string, ensureConsumerMetaData, updateLabel bool) (md *coresecrets.SecretMetadata, err error) {
	notFoundErr := errors.NotFoundf("secret %q", uri)
	if label != "" {
		notFoundErr = errors.NotFoundf("secret with label %q", label)
	}
	defer func() {
		if md == nil {
			return
		}
		if updateLabel {
			if err = s.updateLabelForAppOwnedOrUnitOwnedSecret(uri, label, md); err != nil {
				return
			}
		}
		if md.OwnerTag == s.authTag.String() || !ensureConsumerMetaData {
			return
		}
		md, err = s.ensureConsumerMetadataForAppOwnedSecretsForPeerUnits(md)
	}()

	filter := state.SecretsFilter{
		OwnerTags: []names.Tag{s.authTag},
	}
	if s.authTag.Kind() == names.UnitTagKind {
		// Units can access application owned secrets.
		appOwner := names.NewApplicationTag(commonsecrets.AuthTagApp(s.authTag))
		filter.OwnerTags = append(filter.OwnerTags, appOwner)
	}
	mds, err := s.secretsState.ListSecrets(filter)
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

func (s *SecretsManagerAPI) getSecretContent(arg params.GetSecretContentArg) (
	*secrets.ContentParams, *secretsprovider.ModelBackendConfig, bool, error,
) {
	// Only the owner can access secrets via the secret metadata label added by the owner.
	// (Note: the leader unit is not the owner of the application secrets).
	// Consumers get to use their own label.
	// Both owners and consumers can also just use the secret URI.

	if arg.URI == "" && arg.Label == "" {
		return nil, nil, false, errors.NewNotValid(nil, "both uri and label are empty")
	}

	var uri *coresecrets.URI
	var err error

	if arg.URI != "" {
		uri, err = coresecrets.ParseURI(arg.URI)
		if err != nil {
			return nil, nil, false, errors.Trace(err)
		}
	}

	// arg.Label could be the consumer label for consumers or the owner label for owners.
	possibleUpdateLabel := arg.Label != "" && uri != nil

	// For local secrets, check those which may be owned by the caller.
	if uri == nil || uri.IsLocal(s.modelUUID) {
		// Owner units should always have the URI because we resolved the label to URI on uniter side already.
		md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(uri, arg.Label, true, possibleUpdateLabel)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, nil, false, errors.Trace(err)
		}
		if md != nil {
			// 1. secrets can be accessed by the owner;
			// 2. application owned secrets can be accessed by all the units of the application using owner label or URI.
			val, valueRef, err := s.secretsState.GetSecretValue(md.URI, md.LatestRevision)
			if err != nil {
				return nil, nil, false, errors.Trace(err)
			}
			content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
			if err != nil || content.ValueRef == nil {
				return content, nil, false, errors.Trace(err)
			}
			backend, draining, err := s.getBackend(content.ValueRef.BackendID)
			return content, backend, draining, errors.Trace(err)
		}
	}

	if uri == nil {
		var err error
		uri, err = s.secretsConsumer.GetURIByConsumerLabel(arg.Label, s.authTag)
		if errors.Is(err, errors.NotFound) {
			return nil, nil, false, errors.NotFoundf("consumer label %q", arg.Label)
		}
		if err != nil {
			return nil, nil, false, errors.Trace(err)
		}
	}
	logger.Debugf("getting secret content for: %s", uri)

	if !uri.IsLocal(s.modelUUID) {
		return s.getRemoteSecretContent(uri, arg.Refresh, arg.Peek, arg.Label, possibleUpdateLabel)
	}

	if _, err := s.secretsState.GetSecret(uri); err != nil {
		// Check if the uri exists or not.
		return nil, nil, false, errors.Trace(err)
	}
	if !s.canRead(uri, s.authTag) {
		return nil, nil, false, apiservererrors.ErrPerm
	}

	// arg.Label is the consumer label for consumers.
	consumedRevision, err := s.getConsumedRevision(uri, arg.Refresh, arg.Peek, arg.Label, possibleUpdateLabel)
	if err != nil {
		return nil, nil, false, errors.Annotate(err, "getting latest secret revision")
	}

	val, valueRef, err := s.secretsState.GetSecretValue(uri, consumedRevision)
	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, false, errors.Trace(err)
	}
	backend, draining, err := s.getBackend(content.ValueRef.BackendID)
	return content, backend, draining, errors.Trace(err)
}

func (s *SecretsManagerAPI) getConsumedRevision(uri *coresecrets.URI, refresh, peek bool, label string, possibleUpdateLabel bool) (int, error) {
	consumerInfo, err := s.secretsConsumer.GetSecretConsumer(uri, s.authTag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return 0, errors.Trace(err)
	}
	refresh = refresh ||
		err != nil // Not found, so need to create one.

	var wantRevision int
	if err == nil {
		wantRevision = consumerInfo.CurrentRevision
	}

	// Use the latest revision as the current one if --refresh or --peek.
	if refresh || peek {
		md, err := s.secretsState.GetSecret(uri)
		if err != nil {
			return 0, errors.Trace(err)
		}
		if consumerInfo == nil {
			consumerInfo = &coresecrets.SecretConsumerMetadata{}
		}
		consumerInfo.LatestRevision = md.LatestRevision
		if refresh {
			consumerInfo.CurrentRevision = md.LatestRevision
		}
		wantRevision = md.LatestRevision
	}
	// Save the latest consumer info if required.
	if refresh || possibleUpdateLabel {
		if label != "" {
			consumerInfo.Label = label
		}
		if err := s.secretsConsumer.SaveSecretConsumer(uri, s.authTag, consumerInfo); err != nil {
			return 0, errors.Trace(err)
		}
	}
	return wantRevision, nil
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
		if !commonsecrets.IsSameApplication(s.authTag, tag) {
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
		if !commonsecrets.IsSameApplication(s.authTag, ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		// Only unit leaders can watch application secrets.
		// TODO(wallyworld) - temp fix for old podspec charms
		if ownerTag.Kind() == names.ApplicationTagKind && s.authTag.Kind() != names.ApplicationTagKind {
			_, err := commonsecrets.LeadershipToken(s.authTag, s.leadershipChecker)
			if err != nil {
				return result, errors.Trace(err)
			}
		}
		owners[i] = ownerTag
	}
	w, err := s.secretsState.WatchObsolete(owners)
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
		if !commonsecrets.IsSameApplication(s.authTag, ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		// Only unit leaders can watch application secrets.
		// TODO(wallyworld) - temp fix for old podspec charms
		if ownerTag.Kind() == names.ApplicationTagKind && s.authTag.Kind() != names.ApplicationTagKind {
			_, err := commonsecrets.LeadershipToken(s.authTag, s.leadershipChecker)
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
				URI:             c.URI.ID,
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
		md, err := s.secretsState.GetSecret(uri)
		if err != nil {
			return errors.Trace(err)
		}
		owner, err := names.ParseTag(md.OwnerTag)
		if err != nil {
			return errors.Trace(err)
		}
		if commonsecrets.AuthTagApp(s.authTag) != owner.Id() {
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
		logger.Debugf("secret %q was rotated: rev was %d, now %d", uri.ID, arg.OriginalRevision, md.LatestRevision)
		// If the secret will expire before it is due to be next rotated, rotate sooner to allow
		// the charm a chance to update it before it expires.
		willExpire := md.LatestExpireTime != nil && md.LatestExpireTime.Before(nextRotateTime)
		forcedRotateTime := lastRotateTime.Add(coresecrets.RotateRetryDelay)
		if willExpire {
			logger.Warningf("secret %q rev %d will expire before next scheduled rotation", uri.ID, md.LatestRevision)
		}
		if willExpire && forcedRotateTime.Before(*md.LatestExpireTime) || !arg.Skip && md.LatestRevision == arg.OriginalRevision {
			nextRotateTime = forcedRotateTime
		}
		logger.Debugf("secret %q next rotate time is now: %s", uri.ID, nextRotateTime.UTC().Format(time.RFC3339))
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
		if !commonsecrets.IsSameApplication(s.authTag, ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		// Only unit leaders can watch application secrets.
		// TODO(wallyworld) - temp fix for old podspec charms
		if ownerTag.Kind() == names.ApplicationTagKind && s.authTag.Kind() != names.ApplicationTagKind {
			_, err := commonsecrets.LeadershipToken(s.authTag, s.leadershipChecker)
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
				URI:             c.URI.ID,
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
