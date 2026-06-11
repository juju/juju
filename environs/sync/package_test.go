// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

import (
	"os"
	"testing"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package sync_test -destination simplestreams_mock_test.go github.com/juju/juju/environs/tools SimplestreamsFetcher

func TestMain(m *testing.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
