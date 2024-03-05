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

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
	Criticalf(string, ...interface{})
}
