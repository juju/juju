// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmdownloader

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/core/watcher"
)

// CharmDownloaderAPI describes the API exposed by the charm downloader facade.
type CharmDownloaderAPI interface {
	WatchApplicationsWithPendingCharms(ctx context.Context) (watcher.StringsWatcher, error)
	DownloadApplicationCharms(ctx context.Context, applications []names.ApplicationTag) error
}
