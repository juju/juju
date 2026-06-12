// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader_test

//go:generate go run github.com/canonical/gomock/mockgen -package downloader_test -destination downloadclient_mock_test.go github.com/juju/juju/internal/resource/downloader DownloadClient
