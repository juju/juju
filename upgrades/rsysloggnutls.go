// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"launchpad.net/juju-core/utils"
)

// installRsyslogGnutls installs the rsyslog-gnutls package,
// which is required for our rsyslog configuration from 1.18.0.
func installRsyslogGnutls(context Context) error {
	return utils.AptGetInstall("rsyslog-gnutls")
}
