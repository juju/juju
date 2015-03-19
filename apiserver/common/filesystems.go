// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/poolmanager"
)

type filesystemAlreadyProvisionedError struct {
	error
}

// IsFilesystemAlreadyProvisioned returns true if the specified error
// is caused by a filesystem already being provisioned.
func IsFilesystemAlreadyProvisioned(err error) bool {
	_, ok := err.(*filesystemAlreadyProvisionedError)
	return ok
}

// FilesystemParams returns the parameters for creating the given filesystem.
func FilesystemParams(v state.Filesystem, poolManager poolmanager.PoolManager) (params.FilesystemParams, error) {
	stateFilesystemParams, ok := v.Params()
	if !ok {
		err := &filesystemAlreadyProvisionedError{fmt.Errorf(
			"filesystem %q is already provisioned", v.Tag().Id(),
		)}
		return params.FilesystemParams{}, err
	}

	providerType, cfg, err := StoragePoolConfig(stateFilesystemParams.Pool, poolManager)
	if err != nil {
		return params.FilesystemParams{}, errors.Trace(err)
	}
	return params.FilesystemParams{
		v.Tag().String(),
		stateFilesystemParams.Size,
		string(providerType),
		cfg.Attrs(),
		nil, // attachment params set by the caller
	}, nil
}

// FilesystemFromState converts a state.Filesystem to params.Filesystem.
func FilesystemFromState(v state.Filesystem) (params.Filesystem, error) {
	info, err := v.Info()
	if err != nil {
		return params.Filesystem{}, errors.Trace(err)
	}
	return params.Filesystem{
		v.FilesystemTag().String(),
		info.FilesystemId,
		info.Size,
	}, nil
}

// ParseFilesystemAttachmentIds parses the strings, returning machine storage IDs.
func ParseFilesystemAttachmentIds(stringIds []string) ([]params.MachineStorageId, error) {
	ids := make([]params.MachineStorageId, len(stringIds))
	for i, s := range stringIds {
		m, f, err := state.ParseFilesystemAttachmentId(s)
		if err != nil {
			return nil, err
		}
		ids[i] = params.MachineStorageId{
			MachineTag:    m.String(),
			AttachmentTag: f.String(),
		}
	}
	return ids, nil
}
