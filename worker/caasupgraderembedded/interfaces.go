// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgraderembedded

import (
	"github.com/juju/version/v2"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/api_base_mock.go github.com/juju/juju/api/base APICaller
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/agent_mock.go github.com/juju/juju/agent Agent

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/worker/caasupgraderembedded Logger

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Tracef(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/api_mock.go github.com/juju/juju/worker/caasupgraderembedded UpgraderClient

// UpgraderClient provides the facade methods used by the worker.
type UpgraderClient interface {
	SetVersion(tag string, v version.Binary) error
}
