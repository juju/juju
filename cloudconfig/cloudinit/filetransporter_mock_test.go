// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/cloudconfig/cloudinit (interfaces: FileTransporter)
//
// Generated by this command:
//
//	mockgen -package cloudinit_test -destination filetransporter_mock_test.go github.com/juju/juju/cloudconfig/cloudinit FileTransporter
//

// Package cloudinit_test is a generated GoMock package.
package cloudinit_test

import (
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockFileTransporter is a mock of FileTransporter interface.
type MockFileTransporter struct {
	ctrl     *gomock.Controller
	recorder *MockFileTransporterMockRecorder
}

// MockFileTransporterMockRecorder is the mock recorder for MockFileTransporter.
type MockFileTransporterMockRecorder struct {
	mock *MockFileTransporter
}

// NewMockFileTransporter creates a new mock instance.
func NewMockFileTransporter(ctrl *gomock.Controller) *MockFileTransporter {
	mock := &MockFileTransporter{ctrl: ctrl}
	mock.recorder = &MockFileTransporterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockFileTransporter) EXPECT() *MockFileTransporterMockRecorder {
	return m.recorder
}

// SendBytes mocks base method.
func (m *MockFileTransporter) SendBytes(arg0 string, arg1 []byte) string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendBytes", arg0, arg1)
	ret0, _ := ret[0].(string)
	return ret0
}

// SendBytes indicates an expected call of SendBytes.
func (mr *MockFileTransporterMockRecorder) SendBytes(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendBytes", reflect.TypeOf((*MockFileTransporter)(nil).SendBytes), arg0, arg1)
}
