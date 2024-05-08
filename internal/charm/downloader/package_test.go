// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/logger_mocks.go github.com/juju/juju/internal/charm/downloader Logger
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/charm_mocks.go github.com/juju/juju/internal/charm/downloader CharmArchive
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/charm_archive_mocks.go github.com/juju/juju/internal/charm/downloader CharmRepository
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/storage_mocks.go github.com/juju/juju/internal/charm/downloader Storage
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/repo_mocks.go github.com/juju/juju/internal/charm/downloader RepositoryGetter

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
