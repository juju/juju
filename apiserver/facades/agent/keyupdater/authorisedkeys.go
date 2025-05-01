// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coremachine "github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
)

// KeyUpdaterAPI implements the KeyUpdater interface and is the concrete
// implementation of the api end point.
type KeyUpdaterAPI struct {
	getCanRead        common.GetAuthFunc
	keyUpdaterService KeyUpdaterService
	watcherRegistry   facade.WatcherRegistry
}

// newKeyUpdaterAPI constructs a new [KeyUpdaterAPI] for use in the Juju facade
// model.
func newKeyUpdaterAPI(
	getCanRead common.GetAuthFunc,
	keyUpdaterService KeyUpdaterService,
	watcherRegistry facade.WatcherRegistry,
) *KeyUpdaterAPI {
	return &KeyUpdaterAPI{
		getCanRead:        getCanRead,
		keyUpdaterService: keyUpdaterService,
		watcherRegistry:   watcherRegistry,
	}
}

// WatchAuthorisedKeys starts a watcher to track changes to the authorised ssh
// keys for the specified machines.
// The following param error codes can be expected:
// - [params.CodeTagInvalid] When a tag provided does not parse and is
// considered invalid.
// - [params.CodeTagKindNotSupported] When a tag has been supplied that is not a
// machine tag.
// - [params.CodeUnathorized] When the caller does not have permissions to get
// the authorised keys for a requested tag.
// - [params.CodeMachineInvalidID] When one of the machine tags id is invalid.
func (api *KeyUpdaterAPI) WatchAuthorisedKeys(ctx context.Context, arg params.Entities) (params.NotifyWatchResults, error) {
	results := make([]params.NotifyWatchResult, len(arg.Entities))
	if len(arg.Entities) == 0 {
		return params.NotifyWatchResults{Results: results}, nil
	}

	canRead, err := api.getCanRead(ctx)
	if err != nil {
		return params.NotifyWatchResults{}, fmt.Errorf(
			"checking can read status for key updater watch authorised keys: %w",
			err,
		)
	}

	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagInvalid,
				"cannot parse tag %q: %s",
				entity.Tag,
				err.Error(),
			)
			continue
		}

		if tag.Kind() != names.MachineTagKind {
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagKindNotSupported,
				"tag %q unsupported, can only accept tags of kind %q",
				tag, names.MachineTagKind,
			)
			continue
		}

		if !canRead(tag) {
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeUnauthorized,
				"%q does not have permission to read authorized keys",
				tag,
			)
			continue
		}

		machineId := coremachine.Name(tag.Id())
		keysWatcher, err := api.keyUpdaterService.WatchAuthorisedKeysForMachine(ctx, machineId)
		switch {
		case errors.Is(err, errors.NotValid):
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeMachineInvalidID,
				"invalid machine name %q",
				machineId,
			)
			continue
		case err != nil:
			// We don't understand this error. At this stage we consider it an
			// internal server error and bail out of the call completely.
			return params.NotifyWatchResults{}, fmt.Errorf(
				"cannot watch authorised keys for machine %q: %w",
				machineId, err,
			)
		}

		results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](
			ctx, api.watcherRegistry, keysWatcher,
		)
		if err != nil {
			return params.NotifyWatchResults{}, fmt.Errorf(
				"registering machine %q authorised keys watcher: %w",
				machineId, err,
			)
		}
	}
	return params.NotifyWatchResults{Results: results}, nil
}

// AuthorisedKeys reports the authorised ssh keys for the specified machines.
// The current implementation fetches all keys on the model that have been
// granted for use on a machine.
// The following param error codes can be expected:
// - [params.CodeTagInvalid] When a tag provided does not parse and is
// considered invalid.
// - [params.CodeTagKindNotSupported] When a tag has been supplied that is not a
// machine tag.
// - [params.CodeUnathorized] When the caller does not have permissions to get
// the authorised keys for a requested tag.
// - [params.CodeMachineInvalidID] When one of the machine tag's translates to a
// invalid machine id.
// - [params.CodeMachineNotFound] When one of the machine's does not exist.
func (api *KeyUpdaterAPI) AuthorisedKeys(
	ctx context.Context,
	arg params.Entities,
) (params.StringsResults, error) {
	if len(arg.Entities) == 0 {
		return params.StringsResults{}, nil
	}
	results := make([]params.StringsResult, len(arg.Entities))

	canRead, err := api.getCanRead(ctx)
	if err != nil {
		return params.StringsResults{}, fmt.Errorf(
			"checking can read for authorised keys: %w",
			err,
		)
	}

	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagInvalid,
				"cannot parse tag %q: %s",
				entity.Tag,
				err.Error(),
			)
			continue
		}

		if tag.Kind() != names.MachineTagKind {
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagKindNotSupported,
				"tag %q unsupported, can only accept tags of kind %q",
				tag, names.MachineTagKind,
			)
			continue
		}

		if !canRead(tag) {
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeUnauthorized,
				"no permission to read authorised keys for %q",
				tag,
			)
			continue
		}

		machineName := coremachine.Name(tag.Id())
		keys, err := api.keyUpdaterService.GetAuthorisedKeysForMachine(ctx, machineName)

		switch {
		case errors.Is(err, errors.NotValid):
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeMachineInvalidID,
				"invalid machine name %q",
				machineName,
			)
			continue
		case errors.Is(err, machineerrors.MachineNotFound):
			results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeMachineNotFound,
				"machine %q does not exist",
				machineName,
			)
			continue
		case err != nil:
			// We don't understand this error. At this stage we consider it an
			// internal server error and bail out of the call completely.
			return params.StringsResults{}, fmt.Errorf(
				"cannot get authorised keys for machine %q: %w",
				machineName, err,
			)
		}

		results[i].Result = keys
	}

	return params.StringsResults{
		Results: results,
	}, nil
}
