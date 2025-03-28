// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/apiserver/facades/controller/agenttools (interfaces: ModelConfigService,ModelAgentService)
//
// Generated by this command:
//
//	mockgen -typed -package agenttools -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/agenttools ModelConfigService,ModelAgentService
//

// Package agenttools is a generated GoMock package.
package agenttools

import (
	context "context"
	reflect "reflect"

	semversion "github.com/juju/juju/core/semversion"
	config "github.com/juju/juju/environs/config"
	gomock "go.uber.org/mock/gomock"
)

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
func (mr *MockModelConfigServiceMockRecorder) ModelConfig(arg0 any) *MockModelConfigServiceModelConfigCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ModelConfig", reflect.TypeOf((*MockModelConfigService)(nil).ModelConfig), arg0)
	return &MockModelConfigServiceModelConfigCall{Call: call}
}

// MockModelConfigServiceModelConfigCall wrap *gomock.Call
type MockModelConfigServiceModelConfigCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockModelConfigServiceModelConfigCall) Return(arg0 *config.Config, arg1 error) *MockModelConfigServiceModelConfigCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockModelConfigServiceModelConfigCall) Do(f func(context.Context) (*config.Config, error)) *MockModelConfigServiceModelConfigCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockModelConfigServiceModelConfigCall) DoAndReturn(f func(context.Context) (*config.Config, error)) *MockModelConfigServiceModelConfigCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockModelAgentService is a mock of ModelAgentService interface.
type MockModelAgentService struct {
	ctrl     *gomock.Controller
	recorder *MockModelAgentServiceMockRecorder
}

// MockModelAgentServiceMockRecorder is the mock recorder for MockModelAgentService.
type MockModelAgentServiceMockRecorder struct {
	mock *MockModelAgentService
}

// NewMockModelAgentService creates a new mock instance.
func NewMockModelAgentService(ctrl *gomock.Controller) *MockModelAgentService {
	mock := &MockModelAgentService{ctrl: ctrl}
	mock.recorder = &MockModelAgentServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockModelAgentService) EXPECT() *MockModelAgentServiceMockRecorder {
	return m.recorder
}

// GetModelTargetAgentVersion mocks base method.
func (m *MockModelAgentService) GetModelTargetAgentVersion(arg0 context.Context) (semversion.Number, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetModelTargetAgentVersion", arg0)
	ret0, _ := ret[0].(semversion.Number)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetModelTargetAgentVersion indicates an expected call of GetModelTargetAgentVersion.
func (mr *MockModelAgentServiceMockRecorder) GetModelTargetAgentVersion(arg0 any) *MockModelAgentServiceGetModelTargetAgentVersionCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetModelTargetAgentVersion", reflect.TypeOf((*MockModelAgentService)(nil).GetModelTargetAgentVersion), arg0)
	return &MockModelAgentServiceGetModelTargetAgentVersionCall{Call: call}
}

// MockModelAgentServiceGetModelTargetAgentVersionCall wrap *gomock.Call
type MockModelAgentServiceGetModelTargetAgentVersionCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockModelAgentServiceGetModelTargetAgentVersionCall) Return(arg0 semversion.Number, arg1 error) *MockModelAgentServiceGetModelTargetAgentVersionCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockModelAgentServiceGetModelTargetAgentVersionCall) Do(f func(context.Context) (semversion.Number, error)) *MockModelAgentServiceGetModelTargetAgentVersionCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockModelAgentServiceGetModelTargetAgentVersionCall) DoAndReturn(f func(context.Context) (semversion.Number, error)) *MockModelAgentServiceGetModelTargetAgentVersionCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
