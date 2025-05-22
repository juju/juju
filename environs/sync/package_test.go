// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"os"
	stdtesting "testing"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package sync_test -destination simplestreams_mock_test.go github.com/juju/juju/environs/tools SimplestreamsFetcher

func TestMain(m *stdtesting.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
