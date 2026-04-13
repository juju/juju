// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	context "context"
	reflect "reflect"

	storage "github.com/juju/juju/domain/storage"
	gomock "go.uber.org/mock/gomock"
)

// MockStoragePoolService is a mock of StoragePoolService interface.
type MockStoragePoolService struct {
	ctrl     *gomock.Controller
	recorder *MockStoragePoolServiceMockRecorder
}

// MockStoragePoolServiceMockRecorder is the mock recorder for
// MockStoragePoolService.
type MockStoragePoolServiceMockRecorder struct {
	mock *MockStoragePoolService
}

// NewMockStoragePoolService creates a new mock instance.
func NewMockStoragePoolService(ctrl *gomock.Controller) *MockStoragePoolService {
	mock := &MockStoragePoolService{ctrl: ctrl}
	mock.recorder = &MockStoragePoolServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStoragePoolService) EXPECT() *MockStoragePoolServiceMockRecorder {
	return m.recorder
}

// GetStoragePoolUUID mocks base method.
func (m *MockStoragePoolService) GetStoragePoolUUID(
	arg0 context.Context,
	arg1 string,
) (storage.StoragePoolUUID, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetStoragePoolUUID", arg0, arg1)
	ret0, _ := ret[0].(storage.StoragePoolUUID)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetStoragePoolUUID indicates an expected call of GetStoragePoolUUID.
func (mr *MockStoragePoolServiceMockRecorder) GetStoragePoolUUID(
	arg0, arg1 any,
) *MockStoragePoolServiceGetStoragePoolUUIDCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"GetStoragePoolUUID",
		reflect.TypeOf((*MockStoragePoolService)(nil).GetStoragePoolUUID),
		arg0,
		arg1,
	)
	return &MockStoragePoolServiceGetStoragePoolUUIDCall{Call: call}
}

// MockStoragePoolServiceGetStoragePoolUUIDCall wraps *gomock.Call.
type MockStoragePoolServiceGetStoragePoolUUIDCall struct {
	*gomock.Call
}

// Return rewrites *gomock.Call.Return.
func (c *MockStoragePoolServiceGetStoragePoolUUIDCall) Return(
	arg0 storage.StoragePoolUUID,
	arg1 error,
) *MockStoragePoolServiceGetStoragePoolUUIDCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrites *gomock.Call.Do.
func (c *MockStoragePoolServiceGetStoragePoolUUIDCall) Do(
	f func(context.Context, string) (storage.StoragePoolUUID, error),
) *MockStoragePoolServiceGetStoragePoolUUIDCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrites *gomock.Call.DoAndReturn.
func (c *MockStoragePoolServiceGetStoragePoolUUIDCall) DoAndReturn(
	f func(context.Context, string) (storage.StoragePoolUUID, error),
) *MockStoragePoolServiceGetStoragePoolUUIDCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
