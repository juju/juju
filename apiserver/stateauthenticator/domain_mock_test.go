// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/apiserver/stateauthenticator (interfaces: ControllerConfigGetter)

// Package stateauthenticator_test is a generated GoMock package.
package stateauthenticator_test

import (
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
	controller "github.com/juju/juju/controller"
)

// MockControllerConfigGetter is a mock of ControllerConfigGetter interface.
type MockControllerConfigGetter struct {
	ctrl     *gomock.Controller
	recorder *MockControllerConfigGetterMockRecorder
}

// MockControllerConfigGetterMockRecorder is the mock recorder for MockControllerConfigGetter.
type MockControllerConfigGetterMockRecorder struct {
	mock *MockControllerConfigGetter
}

// NewMockControllerConfigGetter creates a new mock instance.
func NewMockControllerConfigGetter(ctrl *gomock.Controller) *MockControllerConfigGetter {
	mock := &MockControllerConfigGetter{ctrl: ctrl}
	mock.recorder = &MockControllerConfigGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockControllerConfigGetter) EXPECT() *MockControllerConfigGetterMockRecorder {
	return m.recorder
}

// ControllerConfig mocks base method.
func (m *MockControllerConfigGetter) ControllerConfig(arg0 context.Context) (controller.Config, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ControllerConfig", arg0)
	ret0, _ := ret[0].(controller.Config)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ControllerConfig indicates an expected call of ControllerConfig.
func (mr *MockControllerConfigGetterMockRecorder) ControllerConfig(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ControllerConfig", reflect.TypeOf((*MockControllerConfigGetter)(nil).ControllerConfig), arg0)
}