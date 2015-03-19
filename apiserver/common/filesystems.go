// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

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
