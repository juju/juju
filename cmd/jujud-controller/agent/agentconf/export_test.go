// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconf

import (
	"github.com/juju/juju/agent"
)

func NewAgentConfForTest(dataDir string, cfg agent.ConfigSetterWriter) AgentConf {
	return &agentConf{dataDir: dataDir, _config: cfg}
}
