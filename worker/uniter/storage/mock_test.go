// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
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

type mockOperations struct {
	operation.Factory
}

func (m *mockOperations) NewUpdateStorage(tags []names.StorageTag) (operation.Operation, error) {
	return &mockOperation{"update storage"}, nil
}

func (m *mockOperations) NewRunHook(hookInfo hook.Info) (operation.Operation, error) {
	return &mockOperation{fmt.Sprintf("run hook %v", hookInfo.Kind)}, nil
}

type mockOperation struct {
	name string
}

func (m *mockOperation) String() string {
	return m.name
}

func (m *mockOperation) NeedsGlobalMachineLock() bool {
	return false
}

func (m *mockOperation) Prepare(state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Execute(state operation.State) (*operation.State, error) {
	return &state, nil
}

func (m *mockOperation) Commit(state operation.State) (*operation.State, error) {
	return &state, nil
}
