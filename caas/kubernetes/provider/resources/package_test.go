// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"testing"

	"github.com/juju/tc"
)

func Test(t *testing.T) {
	tc.TestingT(t)
}

type (
	Operation = operation
)

var (
	OpApply  opType = opApply
	OpDelete opType = opDelete
)

type ApplierForTest interface {
	Applier
	Operations() []Operation
}

func NewApplierForTest() ApplierForTest {
	return &applier{}
}

func (a *applier) Operations() []Operation {
	return a.ops
}
