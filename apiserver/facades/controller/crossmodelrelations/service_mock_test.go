// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/apiserver/facades/controller/crossmodelrelations (interfaces: ModelConfigService,StatusService)
//
// Generated by this command:
//
//	mockgen -typed -package crossmodelrelations_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/crossmodelrelations ModelConfigService,StatusService
//

// Package crossmodelrelations_test is a generated GoMock package.
package crossmodelrelations_test

import (
	context "context"
	reflect "reflect"

	status "github.com/juju/juju/core/status"
	watcher "github.com/juju/juju/core/watcher"
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

// Watch mocks base method.
func (m *MockModelConfigService) Watch() (watcher.Watcher[[]string], error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Watch")
	ret0, _ := ret[0].(watcher.Watcher[[]string])
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Watch indicates an expected call of Watch.
func (mr *MockModelConfigServiceMockRecorder) Watch() *MockModelConfigServiceWatchCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Watch", reflect.TypeOf((*MockModelConfigService)(nil).Watch))
	return &MockModelConfigServiceWatchCall{Call: call}
}

// MockModelConfigServiceWatchCall wrap *gomock.Call
type MockModelConfigServiceWatchCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockModelConfigServiceWatchCall) Return(arg0 watcher.Watcher[[]string], arg1 error) *MockModelConfigServiceWatchCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockModelConfigServiceWatchCall) Do(f func() (watcher.Watcher[[]string], error)) *MockModelConfigServiceWatchCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockModelConfigServiceWatchCall) DoAndReturn(f func() (watcher.Watcher[[]string], error)) *MockModelConfigServiceWatchCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// MockStatusService is a mock of StatusService interface.
type MockStatusService struct {
	ctrl     *gomock.Controller
	recorder *MockStatusServiceMockRecorder
}

// MockStatusServiceMockRecorder is the mock recorder for MockStatusService.
type MockStatusServiceMockRecorder struct {
	mock *MockStatusService
}

// NewMockStatusService creates a new mock instance.
func NewMockStatusService(ctrl *gomock.Controller) *MockStatusService {
	mock := &MockStatusService{ctrl: ctrl}
	mock.recorder = &MockStatusServiceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStatusService) EXPECT() *MockStatusServiceMockRecorder {
	return m.recorder
}

// GetApplicationDisplayStatus mocks base method.
func (m *MockStatusService) GetApplicationDisplayStatus(arg0 context.Context, arg1 string) (status.StatusInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetApplicationDisplayStatus", arg0, arg1)
	ret0, _ := ret[0].(status.StatusInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetApplicationDisplayStatus indicates an expected call of GetApplicationDisplayStatus.
func (mr *MockStatusServiceMockRecorder) GetApplicationDisplayStatus(arg0, arg1 any) *MockStatusServiceGetApplicationDisplayStatusCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetApplicationDisplayStatus", reflect.TypeOf((*MockStatusService)(nil).GetApplicationDisplayStatus), arg0, arg1)
	return &MockStatusServiceGetApplicationDisplayStatusCall{Call: call}
}

// MockStatusServiceGetApplicationDisplayStatusCall wrap *gomock.Call
type MockStatusServiceGetApplicationDisplayStatusCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStatusServiceGetApplicationDisplayStatusCall) Return(arg0 status.StatusInfo, arg1 error) *MockStatusServiceGetApplicationDisplayStatusCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStatusServiceGetApplicationDisplayStatusCall) Do(f func(context.Context, string) (status.StatusInfo, error)) *MockStatusServiceGetApplicationDisplayStatusCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStatusServiceGetApplicationDisplayStatusCall) DoAndReturn(f func(context.Context, string) (status.StatusInfo, error)) *MockStatusServiceGetApplicationDisplayStatusCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
