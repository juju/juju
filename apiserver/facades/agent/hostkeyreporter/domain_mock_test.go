// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/apiserver/facades/agent/hostkeyreporter (interfaces: MachineService)
//
// Generated by this command:
//
//	mockgen -typed -package hostkeyreporter -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/hostkeyreporter MachineService
//

// Package hostkeyreporter is a generated GoMock package.
package hostkeyreporter

import (
	context "context"
	reflect "reflect"

	machine "github.com/juju/juju/core/machine"
	gomock "go.uber.org/mock/gomock"
)

// MockMachineService is a mock of MachineService interface.
type MockMachineService struct {
	ctrl     *gomock.Controller
	recorder *MockMachineServiceMockRecorder
}

// MockMachineServiceMockRecorder is the mock recorder for MockMachineService.
type MockMachineServiceMockRecorder struct {
	mock *MockMachineService
}

// NewMockMachineService creates a new mock instance.
func NewMockMachineService(ctrl *gomock.Controller) *MockMachineService {
	mock := &MockMachineService{ctrl: ctrl}
	mock.recorder = &MockMachineServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMachineService) EXPECT() *MockMachineServiceMockRecorder {
	return m.recorder
}

// GetMachineUUID mocks base method.
func (m *MockMachineService) GetMachineUUID(arg0 context.Context, arg1 machine.Name) (machine.UUID, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMachineUUID", arg0, arg1)
	ret0, _ := ret[0].(machine.UUID)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetMachineUUID indicates an expected call of GetMachineUUID.
func (mr *MockMachineServiceMockRecorder) GetMachineUUID(arg0, arg1 any) *MockMachineServiceGetMachineUUIDCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMachineUUID", reflect.TypeOf((*MockMachineService)(nil).GetMachineUUID), arg0, arg1)
	return &MockMachineServiceGetMachineUUIDCall{Call: call}
}

// MockMachineServiceGetMachineUUIDCall wrap *gomock.Call
type MockMachineServiceGetMachineUUIDCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockMachineServiceGetMachineUUIDCall) Return(arg0 machine.UUID, arg1 error) *MockMachineServiceGetMachineUUIDCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockMachineServiceGetMachineUUIDCall) Do(f func(context.Context, machine.Name) (machine.UUID, error)) *MockMachineServiceGetMachineUUIDCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockMachineServiceGetMachineUUIDCall) DoAndReturn(f func(context.Context, machine.Name) (machine.UUID, error)) *MockMachineServiceGetMachineUUIDCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// SetSSHHostKeys mocks base method.
func (m *MockMachineService) SetSSHHostKeys(arg0 context.Context, arg1 machine.UUID, arg2 []string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetSSHHostKeys", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetSSHHostKeys indicates an expected call of SetSSHHostKeys.
func (mr *MockMachineServiceMockRecorder) SetSSHHostKeys(arg0, arg1, arg2 any) *MockMachineServiceSetSSHHostKeysCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetSSHHostKeys", reflect.TypeOf((*MockMachineService)(nil).SetSSHHostKeys), arg0, arg1, arg2)
	return &MockMachineServiceSetSSHHostKeysCall{Call: call}
}

// MockMachineServiceSetSSHHostKeysCall wrap *gomock.Call
type MockMachineServiceSetSSHHostKeysCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockMachineServiceSetSSHHostKeysCall) Return(arg0 error) *MockMachineServiceSetSSHHostKeysCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockMachineServiceSetSSHHostKeysCall) Do(f func(context.Context, machine.UUID, []string) error) *MockMachineServiceSetSSHHostKeysCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockMachineServiceSetSSHHostKeysCall) DoAndReturn(f func(context.Context, machine.UUID, []string) error) *MockMachineServiceSetSSHHostKeysCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
