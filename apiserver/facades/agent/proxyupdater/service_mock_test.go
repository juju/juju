// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/apiserver/facades/agent/proxyupdater (interfaces: ControllerNodeService,ModelConfigService)
//
// Generated by this command:
//
//	mockgen -package proxyupdater_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/proxyupdater ControllerNodeService,ModelConfigService
//

// Package proxyupdater_test is a generated GoMock package.
package proxyupdater_test

import (
	context "context"
	reflect "reflect"

	watcher "github.com/juju/juju/core/watcher"
	config "github.com/juju/juju/environs/config"
	gomock "go.uber.org/mock/gomock"
)

// MockControllerNodeService is a mock of ControllerNodeService interface.
type MockControllerNodeService struct {
	ctrl     *gomock.Controller
	recorder *MockControllerNodeServiceMockRecorder
}

// MockControllerNodeServiceMockRecorder is the mock recorder for MockControllerNodeService.
type MockControllerNodeServiceMockRecorder struct {
	mock *MockControllerNodeService
}

// NewMockControllerNodeService creates a new mock instance.
func NewMockControllerNodeService(ctrl *gomock.Controller) *MockControllerNodeService {
	mock := &MockControllerNodeService{ctrl: ctrl}
	mock.recorder = &MockControllerNodeServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockControllerNodeService) EXPECT() *MockControllerNodeServiceMockRecorder {
	return m.recorder
}

// GetAllNoProxyAPIAddressesForAgents mocks base method.
func (m *MockControllerNodeService) GetAllNoProxyAPIAddressesForAgents(arg0 context.Context) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAllNoProxyAPIAddressesForAgents", arg0)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAllNoProxyAPIAddressesForAgents indicates an expected call of GetAllNoProxyAPIAddressesForAgents.
func (mr *MockControllerNodeServiceMockRecorder) GetAllNoProxyAPIAddressesForAgents(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAllNoProxyAPIAddressesForAgents", reflect.TypeOf((*MockControllerNodeService)(nil).GetAllNoProxyAPIAddressesForAgents), arg0)
}

// WatchControllerAPIAddresses mocks base method.
func (m *MockControllerNodeService) WatchControllerAPIAddresses(arg0 context.Context) (watcher.Watcher[struct{}], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WatchControllerAPIAddresses", arg0)
	ret0, _ := ret[0].(watcher.Watcher[struct{}])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// WatchControllerAPIAddresses indicates an expected call of WatchControllerAPIAddresses.
func (mr *MockControllerNodeServiceMockRecorder) WatchControllerAPIAddresses(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WatchControllerAPIAddresses", reflect.TypeOf((*MockControllerNodeService)(nil).WatchControllerAPIAddresses), arg0)
}

// MockModelConfigService is a mock of ModelConfigService interface.
type MockModelConfigService struct {
	ctrl     *gomock.Controller
	recorder *MockModelConfigServiceMockRecorder
}

// MockModelConfigServiceMockRecorder is the mock recorder for MockModelConfigService.
type MockModelConfigServiceMockRecorder struct {
	mock *MockModelConfigService
}

// NewMockModelConfigService creates a new mock instance.
func NewMockModelConfigService(ctrl *gomock.Controller) *MockModelConfigService {
	mock := &MockModelConfigService{ctrl: ctrl}
	mock.recorder = &MockModelConfigServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockModelConfigService) EXPECT() *MockModelConfigServiceMockRecorder {
	return m.recorder
}

// ModelConfig mocks base method.
func (m *MockModelConfigService) ModelConfig(arg0 context.Context) (*config.Config, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ModelConfig", arg0)
	ret0, _ := ret[0].(*config.Config)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ModelConfig indicates an expected call of ModelConfig.
func (mr *MockModelConfigServiceMockRecorder) ModelConfig(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ModelConfig", reflect.TypeOf((*MockModelConfigService)(nil).ModelConfig), arg0)
}

// Watch mocks base method.
func (m *MockModelConfigService) Watch() (watcher.Watcher[[]string], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Watch")
	ret0, _ := ret[0].(watcher.Watcher[[]string])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Watch indicates an expected call of Watch.
func (mr *MockModelConfigServiceMockRecorder) Watch() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Watch", reflect.TypeOf((*MockModelConfigService)(nil).Watch))
}
