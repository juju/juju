// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/canonical/sqlair"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type preparer struct{}

func (p preparer) Prepare(query string, args ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, args...)
}
