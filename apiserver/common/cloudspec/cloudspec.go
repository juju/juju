// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corewatcher "github.com/juju/juju/core/watcher"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// CloudSpecer defines the CloudSpec api interface
type CloudSpecer interface {
	// WatchCloudSpecsChanges returns a watcher for cloud spec changes.
	WatchCloudSpecsChanges(context.Context, params.Entities) (params.NotifyWatchResults, error)

	// CloudSpec returns the model's cloud spec.
	CloudSpec(context.Context, params.Entities) (params.CloudSpecResults, error)

	// GetCloudSpec constructs the CloudSpec for a validated and authorized model.
	GetCloudSpec(context.Context, names.ModelTag) params.CloudSpecResult
}

type CloudSpecAPI struct {
	resources facade.Resources

	getCloudSpec                           func(names.ModelTag) (environscloudspec.CloudSpec, error)
	watchCloudSpec                         func(ctx context.Context, tag names.ModelTag) (corewatcher.NotifyWatcher, error)
	watchCloudSpecModelCredentialReference func(tag names.ModelTag) (state.NotifyWatcher, error)
	watchCloudSpecCredentialContent        func(ctx context.Context, tag names.ModelTag) (corewatcher.NotifyWatcher, error)
	getAuthFunc                            common.GetAuthFunc
}

type CloudSpecAPIV2 struct {
	CloudSpecAPI
}

// NewCloudSpec returns a new CloudSpecAPI.
func NewCloudSpec(
	resources facade.Resources,
	getCloudSpec func(names.ModelTag) (environscloudspec.CloudSpec, error),
	watchCloudSpec func(ctx context.Context, tag names.ModelTag) (corewatcher.NotifyWatcher, error),
	watchCloudSpecModelCredentialReference func(tag names.ModelTag) (state.NotifyWatcher, error),
	watchCloudSpecCredentialContent func(ctx context.Context, tag names.ModelTag) (corewatcher.NotifyWatcher, error),
	getAuthFunc common.GetAuthFunc,
) CloudSpecAPI {
	return CloudSpecAPI{
		resources:                              resources,
		getCloudSpec:                           getCloudSpec,
		watchCloudSpec:                         watchCloudSpec,
		watchCloudSpecModelCredentialReference: watchCloudSpecModelCredentialReference,
		watchCloudSpecCredentialContent:        watchCloudSpecCredentialContent,
		getAuthFunc:                            getAuthFunc,
	}
}

func NewCloudSpecV2(
	resources facade.Resources,
	getCloudSpec func(names.ModelTag) (environscloudspec.CloudSpec, error),
	watchCloudSpec func(ctx context.Context, tag names.ModelTag) (corewatcher.NotifyWatcher, error),
	watchCloudSpecModelCredentialReference func(tag names.ModelTag) (state.NotifyWatcher, error),
	watchCloudSpecCredentialContent func(ctx context.Context, tag names.ModelTag) (corewatcher.NotifyWatcher, error),
	getAuthFunc common.GetAuthFunc,
) CloudSpecAPIV2 {
	api := NewCloudSpec(
		resources,
		getCloudSpec,
		watchCloudSpec,
		watchCloudSpecModelCredentialReference,
		watchCloudSpecCredentialContent,
		getAuthFunc,
	)
	return CloudSpecAPIV2{api}
}

// CloudSpec returns the model's cloud spec.
func (s CloudSpecAPI) CloudSpec(ctx context.Context, args params.Entities) (params.CloudSpecResults, error) {
	authFunc, err := s.getAuthFunc()
	if err != nil {
		return params.CloudSpecResults{}, err
	}
	results := params.CloudSpecResults{
		Results: make([]params.CloudSpecResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !authFunc(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		results.Results[i] = s.GetCloudSpec(ctx, tag)
	}
	return results, nil
}

// GetCloudSpec constructs the CloudSpec for a validated and authorized model.
func (s CloudSpecAPI) GetCloudSpec(ctx context.Context, tag names.ModelTag) params.CloudSpecResult {
	var result params.CloudSpecResult
	spec, err := s.getCloudSpec(tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	var paramsCloudCredential *params.CloudCredential
	if spec.Credential != nil && spec.Credential.AuthType() != "" {
		paramsCloudCredential = &params.CloudCredential{
			AuthType:   string(spec.Credential.AuthType()),
			Attributes: spec.Credential.Attributes(),
		}
	}
	result.Result = &params.CloudSpec{
		Type:              spec.Type,
		Name:              spec.Name,
		Region:            spec.Region,
		Endpoint:          spec.Endpoint,
		IdentityEndpoint:  spec.IdentityEndpoint,
		StorageEndpoint:   spec.StorageEndpoint,
		Credential:        paramsCloudCredential,
		CACertificates:    spec.CACertificates,
		SkipTLSVerify:     spec.SkipTLSVerify,
		IsControllerCloud: spec.IsControllerCloud,
	}
	return result
}

// WatchCloudSpecsChanges returns a watcher for cloud spec changes.
func (s CloudSpecAPI) WatchCloudSpecsChanges(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	authFunc, err := s.getAuthFunc()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !authFunc(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		w, err := s.watchCloudSpecChanges(ctx, tag)
		if err == nil {
			results.Results[i] = w
		} else {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// watcherAdaptor adapts a core watcher to a state watcher.
type watcherAdaptor struct {
	corewatcher.NotifyWatcher
}

func (w *watcherAdaptor) Changes() <-chan struct{} {
	return w.NotifyWatcher.Changes()
}

func (w *watcherAdaptor) Stop() error {
	w.NotifyWatcher.Kill()
	return nil
}

func (w *watcherAdaptor) Err() error {
	return w.NotifyWatcher.Wait()
}

func (s CloudSpecAPI) watchCloudSpecChanges(ctx context.Context, tag names.ModelTag) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	cloudWatch, err := s.watchCloudSpec(ctx, tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	credentialReferenceWatch, err := s.watchCloudSpecModelCredentialReference(tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	credentialContentWatch, err := s.watchCloudSpecCredentialContent(ctx, tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	var watch *common.MultiNotifyWatcher
	if credentialContentWatch != nil {
		watch = common.NewMultiNotifyWatcher(&watcherAdaptor{cloudWatch}, credentialReferenceWatch, &watcherAdaptor{credentialContentWatch})
	} else {
		// It's rare but possible that a model does not have a credential.
		// In this case there is no point trying to 'watch' content changes.
		watch = common.NewMultiNotifyWatcher(&watcherAdaptor{cloudWatch}, credentialReferenceWatch)
	}
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = s.resources.Register(watch)
	} else {
		return result, watcher.EnsureErr(watch)
	}
	return result, nil
}
