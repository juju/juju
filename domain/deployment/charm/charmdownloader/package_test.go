// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

//go:generate go run github.com/canonical/gomock/mockgen -package charmdownloader -destination downloader_mock_test.go github.com/juju/juju/domain/deployment/charm/charmdownloader DownloadClient
