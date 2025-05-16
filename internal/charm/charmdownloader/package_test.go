// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package charmdownloader -destination downloader_mock_test.go github.com/juju/juju/internal/charm/charmdownloader DownloadClient

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
