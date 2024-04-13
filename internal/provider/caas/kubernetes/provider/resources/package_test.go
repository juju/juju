// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
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
