// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagerepo_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package imagerepo -destination registry_test.go github.com/juju/juju/docker/imagerepo Registry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
