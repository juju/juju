// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import "github.com/juju/juju/core/config"

type ServiceExposer interface {
	ExposeService(appName string, resourceTags map[string]string, config config.ConfigAttributes) error
	UnexposeService(appName string) error
}
