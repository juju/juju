// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

type (
	ECSEnviron = environ
)

var (
	CloudSpecToAWSConfig = cloudSpecToAWSConfig
	NewEnviron           = newEnviron
)
