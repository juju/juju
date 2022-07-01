// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"testing"

	coretesting "github.com/juju/juju/v2/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package downloader -destination downloader_mocks.go github.com/juju/juju/core/charm/downloader Logger,CharmArchive,CharmRepository,RepositoryGetter,Storage

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
