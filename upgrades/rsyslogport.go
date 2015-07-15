// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
)

func updateRsyslogPort(context Context) error {
	agentConfig := context.AgentConfig()
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return fmt.Errorf("Failed to get MongoInfo")
	}
	// we need to re-open state with a nil policy so we can bypass
	// validation, as the syslog-port is normally immutable
	st, err := state.Open(agentConfig.Environment(), info, mongo.DefaultDialOpts(), nil)
	if err != nil {
		return err
	}
	defer st.Close()
	attrs := map[string]interface{}{
		"syslog-port": config.DefaultSyslogPort,
	}
	return st.UpdateEnvironConfig(attrs, nil, nil)
}
