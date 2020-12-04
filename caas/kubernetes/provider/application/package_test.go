// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

var (
	Int32Ptr              = int32Ptr
	Int64Ptr              = int64Ptr
	BoolPtr               = boolPtr
	StrPtr                = strPtr
	NewApplicationForTest = newApplication
)
