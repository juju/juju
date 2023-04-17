// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the SecretsDrain facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "SecretsDrain"),
	}
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

// GetSecretsToDrain returns metadata for the secrets that need to be drained.
func (c *Client) GetSecretsToDrain() ([]coresecrets.SecretMetadataForDrain, error) {
	var results params.ListSecretResults
	err := c.facade.FacadeCall("GetSecretsToDrain", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]coresecrets.SecretMetadataForDrain, len(results.Results))
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
		out[i] = coresecrets.SecretMetadataForDrain{Metadata: md, Revisions: revisions}
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
