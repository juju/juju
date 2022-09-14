// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package conv2state

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// StateConverterAPI describes the API exposed by the state converter facade.
type StateConverterAPI interface {
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

// Machiner
type Machiner interface {
	Machine(tag names.MachineTag) (Machine, error)
}

// Machine is a type that has a list of jobs and can be watched.
type Machine interface {
	Jobs() (*params.JobsResult, error)
	Watch() (watcher.NotifyWatcher, error)
}
