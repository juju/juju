// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"go.uber.org/mock/gomock"
)

// TODO (manadart 2018-04-04)
// This, and the shims and mocks in shims.go and shims_mock.go, should be
// phased out.
// The more elegant approach would be to create types that implement the
// methods in the shims by wrapping the calls that are being patched below.
// Then, those types should be passed as dependencies to the objects that
// use them, and can be replaced by mocks in testing.
// See the DBusAPI factory method passed to NewService as an example.

var (
	Serialize = serialize
)

type patcher interface {
	PatchValue(interface{}, interface{})
}

func PatchNewChan(patcher patcher) chan string {
	ch := make(chan string, 1)
	patcher.PatchValue(&newChan, func() chan string { return ch })
	return ch
}

func PatchExec(patcher patcher, ctrl *gomock.Controller) *MockShimExec {
	mock := NewMockShimExec(ctrl)
	patcher.PatchValue(&runCommands, mock.RunCommands)
	return mock
}
