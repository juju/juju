// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"launchpad.net/juju-core/environs/config"
)

func updateRsyslogPort(context Context) error {
	st := context.State()
	attrs := map[string]interface{}{
		"syslog-port": config.DefaultSyslogPort,
	}
	return st.UpdateEnvironConfig(attrs, nil, nil)
}
