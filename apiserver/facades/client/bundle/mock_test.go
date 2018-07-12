// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/juju/testing"
)

type mockState struct {
	testing.Stub
}

func newMockState() *mockState {
	st := &mockState{}
	return st
}
