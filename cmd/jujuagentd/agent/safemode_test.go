// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5/dependency"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/semversion"
)

type safeModeEngineSuite struct{}

func TestSafeModeEngineSuite(t *testing.T) {
	tc.Run(t, &safeModeEngineSuite{})
}

func (*safeModeEngineSuite) TestControllerAgentConfigStarts(c *tc.C) {
	config := &fakeMachineConfig{
		dataDir: c.MkDir(),
		logDir:  c.MkDir(),
		tag:     names.NewControllerAgentTag("0"),
		controllerAgentInfo: controller.ControllerAgentInfo{
			Cert:       "controller-cert",
			PrivateKey: "controller-key",
		},
	}
	agent := &SafeModeMachineAgent{
		AgentConfigWriter: &fakeMachineAgentConfigWriter{config: config},
		agentTag:          config.Tag(),
		configChangedVal:  voyeur.NewValue(true),
	}

	engineWorker, err := agent.makeEngineCreator("", semversion.Zero)(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, engineWorker)

	engine, ok := engineWorker.(*dependency.Engine)
	c.Assert(ok, tc.IsTrue)

	for {
		report := engine.Report(c.Context())
		manifolds := report[dependency.KeyManifolds].(map[string]interface{})
		controllerConfig := manifolds["controller-agent-config"].(map[string]interface{})
		if err, ok := controllerConfig[dependency.KeyError]; ok &&
			strings.Contains(fmt.Sprint(err), "nil ReadyUnlocker") {
			c.Fatalf("controller-agent-config did not start: %v", err)
		}
		if controllerConfig[dependency.KeyState] == "started" {
			return
		}
		runtime.Gosched()
	}
}
