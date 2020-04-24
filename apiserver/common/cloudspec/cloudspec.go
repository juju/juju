// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// CloudSpecAPI implements common methods for use by various
// facades for querying the cloud spec of models.
type CloudSpecAPI interface {
	// WatchCloudSpecsChanges returns a watcher for cloud spec changes.
	WatchCloudSpecsChanges(args params.Entities) (params.NotifyWatchResults, error)

	// CloudSpec returns the model's cloud spec.
	CloudSpec(args params.Entities) (params.CloudSpecResults, error)

	// GetCloudSpec constructs the CloudSpec for a validated and authorized model.
	GetCloudSpec(tag names.ModelTag) params.CloudSpecResult
}

type cloudSpecAPI struct {
	resources facade.Resources

	getCloudSpec                           func(names.ModelTag) (environs.CloudSpec, error)
	watchCloudSpec                         func(tag names.ModelTag) (state.NotifyWatcher, error)
	watchCloudSpecModelCredentialReference func(tag names.ModelTag) (state.NotifyWatcher, error)
	watchCloudSpecCredentialContent        func(tag names.ModelTag) (state.NotifyWatcher, error)
	getAuthFunc                            common.GetAuthFunc
}

// NewCloudSpec returns a new CloudSpecAPI.
func NewCloudSpec(
	resources facade.Resources,
	getCloudSpec func(names.ModelTag) (environs.CloudSpec, error),
	watchCloudSpec func(tag names.ModelTag) (state.NotifyWatcher, error),
	watchCloudSpecModelCredentialReference func(tag names.ModelTag) (state.NotifyWatcher, error),
	watchCloudSpecCredentialContent func(tag names.ModelTag) (state.NotifyWatcher, error),
	getAuthFunc common.GetAuthFunc,
) CloudSpecAPI {
	return cloudSpecAPI{resources,
		getCloudSpec,
		watchCloudSpec,
		watchCloudSpecModelCredentialReference,
		watchCloudSpecCredentialContent,
		getAuthFunc}
}

// CloudSpec returns the model's cloud spec.
func (s cloudSpecAPI) CloudSpec(args params.Entities) (params.CloudSpecResults, error) {
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !authFunc(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		results.Results[i] = s.GetCloudSpec(tag)
	}
	return results, nil
}

// GetCloudSpec constructs the CloudSpec for a validated and authorized model.
func (s cloudSpecAPI) GetCloudSpec(tag names.ModelTag) params.CloudSpecResult {
	var result params.CloudSpecResult
	spec, err := s.getCloudSpec(tag)
	if err != nil {
		result.Error = common.ServerError(err)
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
		Type:             spec.Type,
		Name:             spec.Name,
		Region:           spec.Region,
		Endpoint:         spec.Endpoint,
		IdentityEndpoint: spec.IdentityEndpoint,
		StorageEndpoint:  spec.StorageEndpoint,
		Credential:       paramsCloudCredential,
		CACertificates:   spec.CACertificates,
	}
	return result
}

// WatchCloudSpecsChanges returns a watcher for cloud spec changes.
func (s cloudSpecAPI) WatchCloudSpecsChanges(args params.Entities) (params.NotifyWatchResults, error) {
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !authFunc(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		w, err := s.watchCloudSpecChanges(tag)
		if err == nil {
			results.Results[i] = w
		} else {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

func (s *cloudSpecAPI) watchCloudSpecChanges(tag names.ModelTag) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	cloudWatch, err := s.watchCloudSpec(tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	credentialReferenceWatch, err := s.watchCloudSpecModelCredentialReference(tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	credentialContentWatch, err := s.watchCloudSpecCredentialContent(tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	var watch *common.MultiNotifyWatcher
	if credentialContentWatch != nil {
		watch = common.NewMultiNotifyWatcher(cloudWatch, credentialReferenceWatch, credentialContentWatch)
	} else {
		// It's rare but possible that a model does not have a credential.
		// In this case there is no point trying to 'watch' content changes.
		watch = common.NewMultiNotifyWatcher(cloudWatch, credentialReferenceWatch)
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
