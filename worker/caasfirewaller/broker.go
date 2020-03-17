// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import "github.com/juju/juju/core/application"

type ServiceExposer interface {
	ExposeService(appName string, resourceTags map[string]string, config application.ConfigAttributes) error
	UnExposeService(appName string) error
}
