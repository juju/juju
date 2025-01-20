// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package downloader_test -destination downloadclient_mock_test.go github.com/juju/juju/internal/resource/downloader DownloadClient
