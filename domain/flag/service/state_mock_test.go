// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/domain/flag/service (interfaces: State)
//
// Generated by this command:
//
//	mockgen -package service -destination state_mock_test.go github.com/juju/juju/domain/flag/service State
//

// Package service is a generated GoMock package.
package service

import (
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockState is a mock of State interface.
type MockState struct {
	ctrl     *gomock.Controller
	recorder *MockStateMockRecorder
}

// MockStateMockRecorder is the mock recorder for MockState.
type MockStateMockRecorder struct {
	mock *MockState
}

// NewMockState creates a new mock instance.
func NewMockState(ctrl *gomock.Controller) *MockState {
	mock := &MockState{ctrl: ctrl}
	mock.recorder = &MockStateMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockState) EXPECT() *MockStateMockRecorder {
	return m.recorder
}

// GetFlag mocks base method.
func (m *MockState) GetFlag(arg0 context.Context, arg1 string) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetFlag", arg0, arg1)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetFlag indicates an expected call of GetFlag.
func (mr *MockStateMockRecorder) GetFlag(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetFlag", reflect.TypeOf((*MockState)(nil).GetFlag), arg0, arg1)
}

// SetFlag mocks base method.
func (m *MockState) SetFlag(arg0 context.Context, arg1 string, arg2 bool, arg3 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetFlag", arg0, arg1, arg2, arg3)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetFlag indicates an expected call of SetFlag.
func (mr *MockStateMockRecorder) SetFlag(arg0, arg1, arg2, arg3 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetFlag", reflect.TypeOf((*MockState)(nil).SetFlag), arg0, arg1, arg2, arg3)
}