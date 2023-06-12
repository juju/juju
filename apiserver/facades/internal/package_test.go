// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package internal_test -destination watcher_mock_test.go -source=./watcher.go
func TestAll(t *testing.T) {
	gc.TestingT(t)
}
