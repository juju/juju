// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2instancetypes_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	// force import for fetch_instance_types.go
	_ "github.com/aws/aws-sdk-go"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
