// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the SecretsManager facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "SecretsManager"),
	}
}

// GetSecretBackendConfig fetches the config needed to make a secret backend client.
// If backendID is nil, fetch the current active backend config.
func (c *Client) GetSecretBackendConfig(backendID *string) (*provider.ModelBackendConfigInfo, error) {
	var results params.SecretBackendConfigResults

	args := params.SecretBackendArgs{}
	if backendID != nil {
		args.BackendIDs = []string{*backendID}
	}
	err := c.facade.FacadeCall("GetSecretBackendConfigs", args, &results)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err != nil || len(results.Results) == 0 {
		msg := "active secret backend"
		if backendID != nil {
			msg = fmt.Sprintf("external secret backend id %q", *backendID)
		}
		return nil, errors.NotFoundf(msg)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	info := &provider.ModelBackendConfigInfo{
		ActiveID: results.ActiveID,
		Configs:  make(map[string]provider.ModelBackendConfig),
	}
	for id, cfg := range results.Results {
		info.Configs[id] = provider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: cfg.Config.BackendType,
				Config:      cfg.Config.Params,
			},
		}
	}
	return info, nil
}

// GetBackendConfigForDrain fetches the config needed to make a secret backend client for the drain worker.
func (c *Client) GetBackendConfigForDrain(backendID *string) (*provider.ModelBackendConfig, string, error) {
	var result params.SecretBackendConfigResults
	arg := params.SecretBackendArgs{ForDrain: true}
	if backendID != nil {
		arg.BackendIDs = []string{*backendID}
	}
	err := c.facade.FacadeCall("GetSecretBackendConfigs", arg, &result)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, "", errors.Trace(err)
	}
	if len(result.Results) == 0 {
		return nil, "", errors.NotFoundf("no secret backends available")
	}

	for _, cfg := range result.Results {
		return &provider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: cfg.Config.BackendType,
				Config:      cfg.Config.Params,
			},
		}, result.ActiveID, nil
	}
	if backendID != nil {
		return nil, "", errors.NotFoundf("secret backend %q", *backendID)
	}
	return nil, "", errors.NotFoundf("active secret backend")
}

// CreateSecretURIs generates new secret URIs.
func (c *Client) CreateSecretURIs(count int) ([]*coresecrets.URI, error) {
	var results params.StringResults

	if count <= 0 {
		return nil, errors.NotValidf("secret URi count %d", count)
	}
	if err := c.facade.FacadeCall("CreateSecretURIs", params.CreateSecretURIsArg{
		Count: count,
	}, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != count {
		return nil, errors.Errorf("expected %d result(s), got %d", count, n)
	}
	uris := make([]*coresecrets.URI, count)
	for i, s := range results.Results {
		if err := s.Error; err != nil {
			return nil, errors.Trace(err)
		}
		uri, err := coresecrets.ParseURI(s.Result)
		if err != nil {
			return nil, errors.Trace(err)
		}
		uris[i] = uri
	}
	return uris, nil
}

// GetContentInfo returns info about the content of a secret.
func (c *Client) GetContentInfo(uri *coresecrets.URI, label string, refresh, peek bool) (*secrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	arg := params.GetSecretContentArg{
		Label:   label,
		Refresh: refresh,
		Peek:    peek,
	}
	if uri != nil {
		arg.URI = uri.String()
	}

	var results params.SecretContentResults

	if err := c.facade.FacadeCall(
		"GetSecretContentInfo", params.GetSecretContentArgs{Args: []params.GetSecretContentArg{arg}}, &results,
	); err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	return c.processSecretContentResults(results)
}

func (c *Client) processSecretContentResults(results params.SecretContentResults) (*secrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	if n := len(results.Results); n != 1 {
		return nil, nil, false, errors.Errorf("expected 1 result, got %d", n)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, nil, false, apiservererrors.RestoreError(err)
	}
	content := &secrets.ContentParams{}
	var (
		backendConfig *provider.ModelBackendConfig
		draining      bool
	)
	result := results.Results[0]
	contentParams := results.Results[0].Content
	if contentParams.ValueRef != nil {
		content.ValueRef = &coresecrets.ValueRef{
			BackendID:  contentParams.ValueRef.BackendID,
			RevisionID: contentParams.ValueRef.RevisionID,
		}
		if result.BackendConfig == nil {
			return nil, nil, false, errors.Errorf("missing secret backend info for %q", content.ValueRef)
		}
		backendConfig = &provider.ModelBackendConfig{
			ControllerUUID: result.BackendConfig.ControllerUUID,
			ModelUUID:      result.BackendConfig.ModelUUID,
			ModelName:      result.BackendConfig.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: result.BackendConfig.Config.BackendType,
				Config:      result.BackendConfig.Config.Params,
			},
		}
		draining = result.BackendConfig.Draining
	}
	if len(contentParams.Data) > 0 {
		content.SecretValue = coresecrets.NewSecretValue(contentParams.Data)
	}
	return content, backendConfig, draining, nil
}

// GetRevisionContentInfo returns info about the content of a secret revision.
// If pendingDelete is true, the revision is marked for deletion.
func (c *Client) GetRevisionContentInfo(uri *coresecrets.URI, revision int, pendingDelete bool) (*secrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	arg := params.SecretRevisionArg{
		URI:           uri.String(),
		Revisions:     []int{revision},
		PendingDelete: pendingDelete,
	}

	var results params.SecretContentResults

	if err := c.facade.FacadeCall(
		"GetSecretRevisionContentInfo", arg, &results,
	); err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	return c.processSecretContentResults(results)
}

// WatchConsumedSecretsChanges returns a watcher which serves changes to
// secrets payloads for any secrets consumed by the specified unit.
func (c *Client) WatchConsumedSecretsChanges(unitName string) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUnitTag(unitName).String()}},
	}
	err := c.facade.FacadeCall("WatchConsumedSecretsChanges", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (c *Client) WatchObsolete(ownerTags ...names.Tag) (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	args := params.Entities{Entities: make([]params.Entity, len(ownerTags))}
	for i, tag := range ownerTags {
		args.Entities[i] = params.Entity{Tag: tag.String()}
	}
	err := c.facade.FacadeCall("WatchObsolete", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// GetConsumerSecretsRevisionInfo returns the current revision and labels for secrets consumed
// by the specified unit.
func (c *Client) GetConsumerSecretsRevisionInfo(unitName string, uris []string) (map[string]coresecrets.SecretRevisionInfo, error) {
	var results params.SecretConsumerInfoResults
	args := params.GetSecretConsumerInfoArgs{
		ConsumerTag: names.NewUnitTag(unitName).String(),
		URIs:        uris,
	}

	err := c.facade.FacadeCall("GetConsumerSecretsRevisionInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(uris) {
		return nil, errors.Errorf("expected %d result, got %d", len(uris), len(results.Results))
	}
	info := make(map[string]coresecrets.SecretRevisionInfo)
	for i, latest := range results.Results {
		if err := results.Results[i].Error; err != nil {
			// If deleted or now unauthorised, do not report any info for this url.
			if err.Code == params.CodeNotFound || err.Code == params.CodeUnauthorized {
				continue
			}
			return nil, errors.Annotatef(err, "finding latest info for secret %q", uris[i])
		}
		info[uris[i]] = coresecrets.SecretRevisionInfo{
			Revision: latest.Revision,
			Label:    latest.Label,
		}
	}
	return info, err
}

// SecretMetadata returns metadata for the specified secrets.
func (c *Client) SecretMetadata() ([]coresecrets.SecretOwnerMetadata, error) {
	var results params.ListSecretResults
	err := c.facade.FacadeCall("GetSecretMetadata", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []coresecrets.SecretOwnerMetadata
	for _, info := range results.Results {
		uri, err := coresecrets.ParseURI(info.URI)
		if err != nil {
			return nil, errors.NotValidf("secret URI %q", info.URI)
		}
		md := coresecrets.SecretMetadata{
			URI:              uri,
			OwnerTag:         info.OwnerTag,
			Description:      info.Description,
			Label:            info.Label,
			RotatePolicy:     coresecrets.RotatePolicy(info.RotatePolicy),
			LatestRevision:   info.LatestRevision,
			LatestExpireTime: info.LatestExpireTime,
			NextRotateTime:   info.NextRotateTime,
		}
		revisions := make([]int, len(info.Revisions))
		for i, r := range info.Revisions {
			revisions[i] = r.Revision
		}
		result = append(result, coresecrets.SecretOwnerMetadata{
			Metadata:  md,
			Revisions: revisions,
		})
	}
	return result, nil
}

// WatchSecretsRotationChanges returns a watcher which serves changes to
// secrets rotation config for any secrets managed by the specified owner.
func (c *Client) WatchSecretsRotationChanges(ownerTags ...names.Tag) (watcher.SecretTriggerWatcher, error) {
	var result params.SecretTriggerWatchResult
	args := params.Entities{Entities: make([]params.Entity, len(ownerTags))}
	for i, tag := range ownerTags {
		args.Entities[i] = params.Entity{Tag: tag.String()}
	}
	err := c.facade.FacadeCall("WatchSecretsRotationChanges", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewSecretsTriggerWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// SecretRotated records the outcome of rotating a secret.
func (c *Client) SecretRotated(uri string, oldRevision int) error {
	secretUri, err := coresecrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	var results params.ErrorResults
	args := params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              secretUri.String(),
			OriginalRevision: oldRevision,
		}},
	}
	err = c.facade.FacadeCall("SecretsRotated", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// WatchSecretRevisionsExpiryChanges returns a watcher which serves changes to
// secret revision expiry config for any secrets managed by the specified owner.
func (c *Client) WatchSecretRevisionsExpiryChanges(ownerTags ...names.Tag) (watcher.SecretTriggerWatcher, error) {
	var result params.SecretTriggerWatchResult
	args := params.Entities{Entities: make([]params.Entity, len(ownerTags))}
	for i, tag := range ownerTags {
		args.Entities[i] = params.Entity{Tag: tag.String()}
	}
	err := c.facade.FacadeCall("WatchSecretRevisionsExpiryChanges", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewSecretsTriggerWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// SecretRevokeGrantArgs holds the args used to grant or revoke access to a secret.
// To grant access, specify one of ApplicationName or UnitName, plus optionally RelationId.
// To revoke access, specify one of ApplicationName or UnitName.
type SecretRevokeGrantArgs struct {
	ApplicationName *string
	UnitName        *string
	RelationKey     *string
	Role            coresecrets.SecretRole
}

// Grant grants access to the specified secret.
func (c *Client) Grant(uri *coresecrets.URI, p *SecretRevokeGrantArgs) error {
	args := grantRevokeArgsToParams(p, uri)
	var results params.ErrorResults
	err := c.facade.FacadeCall("SecretsGrant", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func grantRevokeArgsToParams(p *SecretRevokeGrantArgs, secretUri *coresecrets.URI) params.GrantRevokeSecretArgs {
	var subjectTag, scopeTag string
	if p.ApplicationName != nil {
		subjectTag = names.NewApplicationTag(*p.ApplicationName).String()
	}
	if p.UnitName != nil {
		subjectTag = names.NewUnitTag(*p.UnitName).String()
	}
	if p.RelationKey != nil {
		scopeTag = names.NewRelationTag(*p.RelationKey).String()
	} else {
		scopeTag = subjectTag
	}
	args := params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         secretUri.String(),
			ScopeTag:    scopeTag,
			SubjectTags: []string{subjectTag},
			Role:        string(p.Role),
		}},
	}
	return args
}

// Revoke revokes access to the specified secret.
func (c *Client) Revoke(uri *coresecrets.URI, p *SecretRevokeGrantArgs) error {
	args := grantRevokeArgsToParams(p, uri)
	var results params.ErrorResults
	err := c.facade.FacadeCall("SecretsRevoke", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}
