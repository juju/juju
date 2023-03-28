// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/logger_mocks.go github.com/juju/juju/core/charm/downloader Logger
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charm_mocks.go github.com/juju/juju/core/charm/downloader CharmArchive,CharmRepository
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/storage_mocks.go github.com/juju/juju/core/charm/downloader RepositoryGetter,Storage

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
