// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider/registry"
)

type volumeAlreadyProvisionedError struct {
	error
}

// IsVolumeAlreadyProvisioned returns true if the specified error
// is caused by a volume already being provisioned.
func IsVolumeAlreadyProvisioned(err error) bool {
	_, ok := err.(*volumeAlreadyProvisionedError)
	return ok
}

// VolumeParams returns the parameters for creating the given volume.
func VolumeParams(v state.Volume, poolManager poolmanager.PoolManager) (params.VolumeParams, error) {
	stateVolumeParams, ok := v.Params()
	if !ok {
		err := &volumeAlreadyProvisionedError{fmt.Errorf(
			"volume %q is already provisioned", v.Tag().Id(),
		)}
		return params.VolumeParams{}, err
	}

	var providerType storage.ProviderType
	var attrs map[string]interface{}
	if pool, err := poolManager.Get(stateVolumeParams.Pool); errors.IsNotFound(err) {
		// If not a storage pool, then maybe a provider type.
		providerType = storage.ProviderType(stateVolumeParams.Pool)
		if _, err1 := registry.StorageProvider(providerType); err1 != nil {
			return params.VolumeParams{}, errors.Trace(err)
		}
	} else if err != nil {
		return params.VolumeParams{}, errors.Annotate(err, "getting pool")
	} else {
		providerType = pool.Provider()
		attrs = pool.Attrs()
	}
	return params.VolumeParams{
		v.Tag().String(),
		stateVolumeParams.Size,
		string(providerType),
		attrs,
		"", // machine tag is set by the machine provisioner
	}, nil
}

// VolumesToState converts a slice of params.Volume to a mapping
// of volume tags to state.VolumeInfo.
func VolumesToState(in []params.Volume) (map[names.VolumeTag]state.VolumeInfo, error) {
	m := make(map[names.VolumeTag]state.VolumeInfo)
	for _, v := range in {
		tag, volumeInfo, err := VolumeToState(v)
		if err != nil {
			return nil, errors.Trace(err)
		}
		m[tag] = volumeInfo
	}
	return m, nil
}

// VolumeToState converts a params.Volume to state.VolumeInfo
// and names.VolumeTag.
func VolumeToState(v params.Volume) (names.VolumeTag, state.VolumeInfo, error) {
	if v.VolumeTag == "" {
		return names.VolumeTag{}, state.VolumeInfo{}, errors.New("Tag is empty")
	}
	volumeTag, err := names.ParseVolumeTag(v.VolumeTag)
	if err != nil {
		return names.VolumeTag{}, state.VolumeInfo{}, errors.Trace(err)
	}
	return volumeTag, state.VolumeInfo{
		v.Serial,
		v.Size,
		v.VolumeId,
	}, nil
}

// VolumeFromState converts a state.Volume to params.Volume.
func VolumeFromState(v state.Volume) (params.Volume, error) {
	info, err := v.Info()
	if err != nil {
		return params.Volume{}, errors.Trace(err)
	}
	return params.Volume{
		v.VolumeTag().String(),
		info.VolumeId,
		info.Serial,
		info.Size,
	}, nil
}

// VolumeAttachmentsToState converts a slice of storage.VolumeAttachment to a
// mapping of volume tags to state.VolumeAttachmentInfo.
func VolumeAttachmentsToState(in []params.VolumeAttachment) (map[names.VolumeTag]state.VolumeAttachmentInfo, error) {
	m := make(map[names.VolumeTag]state.VolumeAttachmentInfo)
	for _, v := range in {
		if v.VolumeTag == "" {
			return nil, errors.New("Tag is empty")
		}
		volumeTag, err := names.ParseVolumeTag(v.VolumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		m[volumeTag] = state.VolumeAttachmentInfo{
			v.DeviceName,
			v.ReadOnly,
		}
	}
	return m, nil
}
