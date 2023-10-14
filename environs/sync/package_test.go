// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"os"
	"testing"

	jujutesting "github.com/juju/testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package sync_test -destination simplestreams_mock_test.go github.com/juju/juju/environs/tools SimplestreamsFetcher

func TestMain(m *testing.M) {
	jujutesting.ExecHelperProcess()
	os.Exit(m.Run())
}
