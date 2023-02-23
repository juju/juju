// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/kubernetes_mocks.go github.com/juju/juju/secrets/provider/kubernetes Broker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
