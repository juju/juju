// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub_test

//go:generate go run github.com/canonical/gomock/mockgen -package charmhub_test -destination charmhub_mock_test.go github.com/juju/juju/internal/resource/charmhub ResourceClient,CharmHub,Downloader
