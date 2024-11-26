// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package downloader -destination downloader_mock_test.go github.com/juju/juju/domain/application/charm/downloader CharmRepository,RepositoryGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
