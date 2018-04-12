// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import "github.com/juju/juju/core/application"

type ServiceExposer interface {
	ExposeService(appName string, config application.ConfigAttributes) error
	UnexposeService(appName string) error
}
