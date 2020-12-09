// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charmhub.go github.com/juju/juju/apiserver/facades/client/resources CharmHub
