// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/core/objectstore (interfaces: ObjectStore)
//
// Generated by this command:
//
//	mockgen -typed -package machinemanager_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//

// Package machinemanager_test is a generated GoMock package.
package machinemanager_test

import (
	context "context"
	io "io"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockObjectStore is a mock of ObjectStore interface.
type MockObjectStore struct {
	ctrl     *gomock.Controller
	recorder *MockObjectStoreMockRecorder
}

// MockObjectStoreMockRecorder is the mock recorder for MockObjectStore.
type MockObjectStoreMockRecorder struct {
	mock *MockObjectStore
}

// NewMockObjectStore creates a new mock instance.
func NewMockObjectStore(ctrl *gomock.Controller) *MockObjectStore {
	mock := &MockObjectStore{ctrl: ctrl}
	mock.recorder = &MockObjectStoreMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockObjectStore) EXPECT() *MockObjectStoreMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockObjectStore) Get(arg0 context.Context, arg1 string) (io.ReadCloser, int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", arg0, arg1)
	ret0, _ := ret[0].(io.ReadCloser)
	ret1, _ := ret[1].(int64)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// Get indicates an expected call of Get.
func (mr *MockObjectStoreMockRecorder) Get(arg0, arg1 any) *MockObjectStoreGetCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockObjectStore)(nil).Get), arg0, arg1)
	return &MockObjectStoreGetCall{Call: call}
}

// MockObjectStoreGetCall wrap *gomock.Call
type MockObjectStoreGetCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockObjectStoreGetCall) Return(arg0 io.ReadCloser, arg1 int64, arg2 error) *MockObjectStoreGetCall {
	c.Call = c.Call.Return(arg0, arg1, arg2)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockObjectStoreGetCall) Do(f func(context.Context, string) (io.ReadCloser, int64, error)) *MockObjectStoreGetCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockObjectStoreGetCall) DoAndReturn(f func(context.Context, string) (io.ReadCloser, int64, error)) *MockObjectStoreGetCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Put mocks base method.
func (m *MockObjectStore) Put(arg0 context.Context, arg1 string, arg2 io.Reader, arg3 int64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Put", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// Put indicates an expected call of Put.
func (mr *MockObjectStoreMockRecorder) Put(arg0, arg1, arg2, arg3 any) *MockObjectStorePutCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Put", reflect.TypeOf((*MockObjectStore)(nil).Put), arg0, arg1, arg2, arg3)
	return &MockObjectStorePutCall{Call: call}
}

// MockObjectStorePutCall wrap *gomock.Call
type MockObjectStorePutCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockObjectStorePutCall) Return(arg0 error) *MockObjectStorePutCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockObjectStorePutCall) Do(f func(context.Context, string, io.Reader, int64) error) *MockObjectStorePutCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockObjectStorePutCall) DoAndReturn(f func(context.Context, string, io.Reader, int64) error) *MockObjectStorePutCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// PutAndCheckHash mocks base method.
func (m *MockObjectStore) PutAndCheckHash(arg0 context.Context, arg1 string, arg2 io.Reader, arg3 int64, arg4 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PutAndCheckHash", arg0, arg1, arg2, arg3, arg4)
	ret0, _ := ret[0].(error)
	return ret0
}

// PutAndCheckHash indicates an expected call of PutAndCheckHash.
func (mr *MockObjectStoreMockRecorder) PutAndCheckHash(arg0, arg1, arg2, arg3, arg4 any) *MockObjectStorePutAndCheckHashCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PutAndCheckHash", reflect.TypeOf((*MockObjectStore)(nil).PutAndCheckHash), arg0, arg1, arg2, arg3, arg4)
	return &MockObjectStorePutAndCheckHashCall{Call: call}
}

// MockObjectStorePutAndCheckHashCall wrap *gomock.Call
type MockObjectStorePutAndCheckHashCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockObjectStorePutAndCheckHashCall) Return(arg0 error) *MockObjectStorePutAndCheckHashCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockObjectStorePutAndCheckHashCall) Do(f func(context.Context, string, io.Reader, int64, string) error) *MockObjectStorePutAndCheckHashCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockObjectStorePutAndCheckHashCall) DoAndReturn(f func(context.Context, string, io.Reader, int64, string) error) *MockObjectStorePutAndCheckHashCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Remove mocks base method.
func (m *MockObjectStore) Remove(arg0 context.Context, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Remove", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Remove indicates an expected call of Remove.
func (mr *MockObjectStoreMockRecorder) Remove(arg0, arg1 any) *MockObjectStoreRemoveCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Remove", reflect.TypeOf((*MockObjectStore)(nil).Remove), arg0, arg1)
	return &MockObjectStoreRemoveCall{Call: call}
}

// MockObjectStoreRemoveCall wrap *gomock.Call
type MockObjectStoreRemoveCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockObjectStoreRemoveCall) Return(arg0 error) *MockObjectStoreRemoveCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockObjectStoreRemoveCall) Do(f func(context.Context, string) error) *MockObjectStoreRemoveCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockObjectStoreRemoveCall) DoAndReturn(f func(context.Context, string) error) *MockObjectStoreRemoveCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}