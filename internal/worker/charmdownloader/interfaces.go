// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/watcher"
)

// CharmDownloaderAPI describes the API exposed by the charm downloader facade.
type CharmDownloaderAPI interface {
	WatchApplicationsWithPendingCharms() (watcher.StringsWatcher, error)
	DownloadApplicationCharms(applications []names.ApplicationTag) error
}
