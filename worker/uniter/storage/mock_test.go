// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

type mockStorageAccessor struct {
	storageAttachment             func(names.StorageTag, names.UnitTag) (params.StorageAttachment, error)
	storageAttachmentLife         func([]params.StorageAttachmentId) ([]params.LifeResult, error)
	unitStorageAttachments        func(names.UnitTag) ([]params.StorageAttachmentId, error)
	destroyUnitStorageAttachments func(names.UnitTag) error
	remove                        func(names.StorageTag, names.UnitTag) error
}

func (m *mockStorageAccessor) StorageAttachment(s names.StorageTag, u names.UnitTag) (params.StorageAttachment, error) {
	return m.storageAttachment(s, u)
}

func (m *mockStorageAccessor) StorageAttachmentLife(ids []params.StorageAttachmentId) ([]params.LifeResult, error) {
	if m.storageAttachmentLife != nil {
		return m.storageAttachmentLife(ids)
	}
	results := make([]params.LifeResult, len(ids))
	for i, id := range ids {
		storageTag, err := names.ParseStorageTag(id.StorageTag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		unitTag, err := names.ParseUnitTag(id.UnitTag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		att, err := m.storageAttachment(storageTag, unitTag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Life = att.Life
	}
	return results, nil
}

func (m *mockStorageAccessor) UnitStorageAttachments(u names.UnitTag) ([]params.StorageAttachmentId, error) {
	return m.unitStorageAttachments(u)
}

func (m *mockStorageAccessor) DestroyUnitStorageAttachments(u names.UnitTag) error {
	return m.destroyUnitStorageAttachments(u)
}

func (m *mockStorageAccessor) RemoveStorageAttachment(s names.StorageTag, u names.UnitTag) error {
	return m.remove(s, u)
}
