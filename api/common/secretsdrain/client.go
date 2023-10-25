// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"context"

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
func NewClient(facade base.FacadeCaller) *Client {
	return &Client{facade: facade}
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
	err := c.facade.FacadeCall(context.TODO(), "GetSecretsToDrain", nil, &results)
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

// ChangeSecretBackendArg is the argument for ChangeSecretBackend.
type ChangeSecretBackendArg struct {
	URI      *coresecrets.URI
	Revision int
	Data     map[string]string
	ValueRef *coresecrets.ValueRef
}

// ChangeSecretBackendResult is the result for ChangeSecretBackend.
type ChangeSecretBackendResult struct {
	Results []error
}

// ErrorCount returns the number of errors in the result.
func (r ChangeSecretBackendResult) ErrorCount() (out int) {
	for _, err := range r.Results {
		if err != nil {
			out++
		}
	}
	return out
}

// ChangeSecretBackend updates the backend for the specified secret after migration done.
func (c *Client) ChangeSecretBackend(metaRevisions []ChangeSecretBackendArg) (ChangeSecretBackendResult, error) {
	var results params.ErrorResults
	out := ChangeSecretBackendResult{Results: make([]error, len(metaRevisions))}
	args := params.ChangeSecretBackendArgs{Args: make([]params.ChangeSecretBackendArg, len(metaRevisions))}
	for i, mdr := range metaRevisions {
		arg := params.ChangeSecretBackendArg{
			URI:      mdr.URI.String(),
			Revision: mdr.Revision,
			Content:  params.SecretContentParams{Data: mdr.Data},
		}
		if mdr.ValueRef != nil {
			arg.Content.ValueRef = &params.SecretValueRef{
				BackendID:  mdr.ValueRef.BackendID,
				RevisionID: mdr.ValueRef.RevisionID,
			}
		}
		args.Args[i] = arg
	}
	err := c.facade.FacadeCall(context.TODO(), "ChangeSecretBackend", args, &results)
	if err != nil {
		return out, errors.Trace(err)
	}
	if len(results.Results) != len(metaRevisions) {
		return out, errors.Errorf("expected %d result, got %d", len(metaRevisions), len(results.Results))
	}
	for i, result := range results.Results {
		out.Results[i] = apiservererrors.RestoreError(result.Error)
	}
	return out, nil
}

// WatchSecretBackendChanged sets up a watcher to notify of changes to the secret backend.
func (c *Client) WatchSecretBackendChanged() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	if err := c.facade.FacadeCall(context.TODO(), "WatchSecretBackendChanged", nil, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
