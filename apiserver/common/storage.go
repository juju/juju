// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type StorageAccessor interface {
	StorageInstance(names.StorageTag) (state.StorageInstance, error)
	UnitStorageAttachments(names.UnitTag) ([]state.StorageAttachment, error)
}

// UnitStorage returns the storage instances attached to the specified unit.
func UnitStorage(st StorageAccessor, unit names.UnitTag) ([]state.StorageInstance, error) {
	attachments, err := st.UnitStorageAttachments(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances := make([]state.StorageInstance, 0, len(attachments))
	for _, attachment := range attachments {
		instance, err := st.StorageInstance(attachment.StorageInstance())
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// ClassifyDetachedStorage classifies storage instances into those that will
// be destroyed, and those that will be detached, when their attachment is
// removed.
func ClassifyDetachedStorage(storage []state.StorageInstance) (destroyed, detached []params.Entity) {
	for _, storage := range storage {
		// TODO(axw) we need to expose on StorageInstance
		// whether or not it is detachable. Then we can
		// decide here whether the storage will be detached
		// or destroyed.
		destroyed = append(destroyed, params.Entity{storage.StorageTag().String()})
	}
	return destroyed, detached
}
