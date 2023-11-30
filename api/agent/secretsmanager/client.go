// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	commonsecretbackends "github.com/juju/juju/api/common/secretbackends"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the SecretsManager facade.
type Client struct {
	facade base.FacadeCaller
	*commonsecretbackends.Client
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *Client {
	facade := base.NewFacadeCaller(caller, "SecretsManager")
	return &Client{
		facade: facade,
		Client: commonsecretbackends.NewClient(facade),
	}
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
		for _, g := range info.Access {
			md.Access = append(md.Access, coresecrets.AccessInfo{
				Target: g.TargetTag, Scope: g.ScopeTag, Role: g.Role,
			})
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
