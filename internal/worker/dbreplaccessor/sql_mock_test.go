// Code generated by MockGen. DO NOT EDIT.
// Source: database/sql/driver (interfaces: Driver)
//
// Generated by this command:
//
//	mockgen -typed -package dbreplaccessor -destination sql_mock_test.go database/sql/driver Driver
//

// Package dbreplaccessor is a generated GoMock package.
package dbreplaccessor

import (
	driver "database/sql/driver"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockDriver is a mock of Driver interface.
type MockDriver struct {
	ctrl     *gomock.Controller
	recorder *MockDriverMockRecorder
}

// MockDriverMockRecorder is the mock recorder for MockDriver.
type MockDriverMockRecorder struct {
	mock *MockDriver
}

// NewMockDriver creates a new mock instance.
func NewMockDriver(ctrl *gomock.Controller) *MockDriver {
	mock := &MockDriver{ctrl: ctrl}
	mock.recorder = &MockDriverMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDriver) EXPECT() *MockDriverMockRecorder {
	return m.recorder
}

// Open mocks base method.
func (m *MockDriver) Open(arg0 string) (driver.Conn, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Open", arg0)
	ret0, _ := ret[0].(driver.Conn)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Open indicates an expected call of Open.
func (mr *MockDriverMockRecorder) Open(arg0 any) *MockDriverOpenCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Open", reflect.TypeOf((*MockDriver)(nil).Open), arg0)
	return &MockDriverOpenCall{Call: call}
}

// MockDriverOpenCall wrap *gomock.Call
type MockDriverOpenCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockDriverOpenCall) Return(arg0 driver.Conn, arg1 error) *MockDriverOpenCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockDriverOpenCall) Do(f func(string) (driver.Conn, error)) *MockDriverOpenCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockDriverOpenCall) DoAndReturn(f func(string) (driver.Conn, error)) *MockDriverOpenCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}