// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
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
func (c *Client) GetSecretBackendConfig() (*provider.ModelBackendConfigInfo, error) {
	var results params.SecretBackendConfigResults
	err := c.facade.FacadeCall("GetSecretBackendConfig", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := &provider.ModelBackendConfigInfo{
		ControllerUUID: results.ControllerUUID,
		ModelUUID:      results.ModelUUID,
		ModelName:      results.ModelName,
		ActiveID:       results.ActiveID,
		Configs:        make(map[string]provider.BackendConfig),
	}
	for id, cfg := range results.Configs {
		info.Configs[id] = provider.BackendConfig{
			BackendType: cfg.BackendType,
			Config:      cfg.Params,
		}
	}
	return info, nil
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
func (c *Client) GetContentInfo(uri *coresecrets.URI, label string, refresh, peek bool) (*secrets.ContentParams, error) {
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
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, apiservererrors.RestoreError(err)
	}
	content := &secrets.ContentParams{}
	result := results.Results[0].Content
	if result.ValueRef != nil {
		content.ValueRef = &coresecrets.ValueRef{
			BackendID:  result.ValueRef.BackendID,
			RevisionID: result.ValueRef.RevisionID,
		}
	}
	if len(result.Data) > 0 {
		content.SecretValue = coresecrets.NewSecretValue(result.Data)
	}
	return content, nil
}

// GetRevisionContentInfo returns info about the content of a secret revision.
// If pendingDelete is true, the revision is marked for deletion.
func (c *Client) GetRevisionContentInfo(uri *coresecrets.URI, revision int, pendingDelete bool) (*secrets.ContentParams, error) {
	arg := params.SecretRevisionArg{
		URI:           uri.String(),
		Revisions:     []int{revision},
		PendingDelete: pendingDelete,
	}

	var results params.SecretContentResults

	if err := c.facade.FacadeCall(
		"GetSecretRevisionContentInfo", arg, &results,
	); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, apiservererrors.RestoreError(err)
	}
	content := &secrets.ContentParams{}
	result := results.Results[0].Content
	if result.ValueRef != nil {
		content.ValueRef = &coresecrets.ValueRef{
			BackendID:  result.ValueRef.BackendID,
			RevisionID: result.ValueRef.RevisionID,
		}
	}
	if len(result.Data) > 0 {
		content.SecretValue = coresecrets.NewSecretValue(result.Data)
	}
	return content, nil
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

func processListSecretResult(info params.ListSecretResult) (out coresecrets.SecretMetadata, _ error) {
	uri, err := coresecrets.ParseURI(info.URI)
	if err != nil {
		return out, errors.NotValidf("secret URI %q", info.URI)
	}
	return coresecrets.SecretMetadata{
		URI:              uri,
		OwnerTag:         info.OwnerTag,
		Description:      info.Description,
		Label:            info.Label,
		RotatePolicy:     coresecrets.RotatePolicy(info.RotatePolicy),
		LatestRevision:   info.LatestRevision,
		LatestExpireTime: info.LatestExpireTime,
		NextRotateTime:   info.NextRotateTime,
	}, nil
}

// SecretMetadata returns metadata for the specified secrets.
func (c *Client) SecretMetadata() ([]coresecrets.SecretOwnerMetadata, error) {
	var results params.ListSecretResults
	err := c.facade.FacadeCall("GetSecretMetadata", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]coresecrets.SecretOwnerMetadata, len(results.Results))
	for i, info := range results.Results {
		md, err := processListSecretResult(info)
		if err != nil {
			return nil, errors.Trace(err)
		}
		revisions := make([]int, len(info.Revisions))
		for i, r := range info.Revisions {
			revisions[i] = r.Revision
		}
		out[i] = coresecrets.SecretOwnerMetadata{Metadata: md, Revisions: revisions}
	}
	return out, nil
}

// GetSecretsToMigrate returns metadata for the secrets that need to be migrated.
func (c *Client) GetSecretsToMigrate() ([]coresecrets.SecretMetadataForMigration, error) {
	var results params.ListSecretResults
	err := c.facade.FacadeCall("GetSecretsToMigrate", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]coresecrets.SecretMetadataForMigration, len(results.Results))
	for i, info := range results.Results {
		md, err := processListSecretResult(info)
		if err != nil {
			return nil, errors.Trace(err)
		}
		revisions := make([]coresecrets.SecretRevisionMetadata, len(info.Revisions))
		for i, r := range info.Revisions {
			rev := coresecrets.SecretRevisionMetadata{
				Revision:    r.Revision,
				BackendName: r.BackendName,
				CreateTime:  r.CreateTime,
				UpdateTime:  r.UpdateTime,
				ExpireTime:  r.ExpireTime,
			}
			if r.ValueRef != nil {
				rev.ValueRef = &coresecrets.ValueRef{
					BackendID:  r.ValueRef.BackendID,
					RevisionID: r.ValueRef.RevisionID,
				}
			}
			revisions[i] = rev
		}
		out[i] = coresecrets.SecretMetadataForMigration{Metadata: md, Revisions: revisions}
	}
	return out, nil
}

// ChangeSecretBackend updates the backend for the specified secret after migration done.
func (c *Client) ChangeSecretBackend(uri *coresecrets.URI, revision int, valueRef *coresecrets.ValueRef, data coresecrets.SecretData) error {
	var results params.ErrorResults
	arg := params.ChangeSecretBackendArg{
		URI:      uri.String(),
		Revision: revision,
		Content:  params.SecretContentParams{Data: data},
	}
	if valueRef != nil {
		arg.Content.ValueRef = &params.SecretValueRef{
			BackendID:  valueRef.BackendID,
			RevisionID: valueRef.RevisionID,
		}
	}
	args := params.ChangeSecretBackendArgs{Args: []params.ChangeSecretBackendArg{arg}}
	err := c.facade.FacadeCall("ChangeSecretBackend", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	return apiservererrors.RestoreError(result.Error)
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

// WatchSecretBackendChanged sets up a watcher to notify of changes to the secret backend.
func (c *Client) WatchSecretBackendChanged() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	if err := c.facade.FacadeCall("WatchSecretBackendChanged", nil, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
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
