// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/downloader_mocks.go github.com/juju/juju/core/charm/downloader Logger,CharmArchive,CharmRepository,RepositoryGetter,Storage

func Test(t *testing.T) {
	gc.TestingT(t)
}
