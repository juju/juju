// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"testing"

	"github.com/juju/tc"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package sync_test -destination simplestreams_mock_test.go github.com/juju/juju/environs/tools SimplestreamsFetcher
