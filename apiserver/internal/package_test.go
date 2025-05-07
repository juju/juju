// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package internal_test -destination watcher_mock_test.go -source=./watcher.go

func TestAll(t *testing.T) {
	tc.TestingT(t)
}
