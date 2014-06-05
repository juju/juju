// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/juju/utils"

	"github.com/juju/juju/tools"
)

var (
	RetryAfter           = &retryAfter
	AllowedTargetVersion = allowedTargetVersion
)

func EnsureTools(u *Upgrader, agentTools *tools.Tools, hostnameVerification utils.SSLHostnameVerification) error {
	return u.ensureTools(agentTools, hostnameVerification)
}
