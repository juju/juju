// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"launchpad.net/juju-core/environs/config"
)

func updateRsyslogPort(context Context) error {
	st := context.State()
	old, err := st.EnvironConfig()
	if err != nil {
		return err
	}
	cfg, err := old.Apply(map[string]interface{}{
		"syslog-port": config.DefaultSyslogPort,
	})
	if err != nil {
		return err
	}
	return st.SetEnvironConfig(cfg, old)
}
