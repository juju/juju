// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

func updateRsyslogPort(context Context) error {
	agentConfig := context.AgentConfig()
	info, ok := agentConfig.StateInfo()
	if !ok {
		return fmt.Errorf("Failed to get StateInfo")
	}
	// we need to re-open state with a nil policay so we can bypass
	// validation, as the syslog-port is normally immutable
	st, err := state.Open(info, state.DefaultDialOpts(), nil)
	if err != nil {
		return err
	}
	defer st.Close()
	attrs := map[string]interface{}{
		"syslog-port": config.DefaultSyslogPort,
	}
	return st.UpdateEnvironConfig(attrs, nil, nil)
}
