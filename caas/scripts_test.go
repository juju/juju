// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas_test

import (
	"fmt"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/testing"
)

type scriptsSuite struct {
	testing.BaseSuite
}

func TestScriptsSuite(t *stdtesting.T) {
	tc.Run(t, &scriptsSuite{})
}

func (s *scriptsSuite) TestModelOperatorStartUpScript(c *tc.C) {
	script := fmt.Sprintf(
		caas.ModelOperatorStartUpSh,
		"/var/lib/juju",
		"tools",
		"exec $JUJU_TOOLS_DIR/jujuagentd model --model-uuid=model-uuid",
		"exec $JUJU_TOOLS_DIR/jujud model --model-uuid=model-uuid",
	)

	// The FHS binaries shipped by the chiselled rock are selected before
	// the /opt paths of the Docker image.
	agentdIndex := strings.Index(script, "[ -x /usr/bin/jujuagentd ]")
	c.Assert(agentdIndex >= 0, tc.IsTrue)
	c.Check(strings.Contains(script,
		"cp /usr/bin/jujuagentd $JUJU_TOOLS_DIR/jujuagentd"), tc.IsTrue)

	optAgentdIndex := strings.Index(script, "[ -x /opt/jujuagentd ]")
	c.Assert(optAgentdIndex >= 0, tc.IsTrue)
	c.Check(agentdIndex < optAgentdIndex, tc.IsTrue)
	c.Check(strings.Contains(script,
		"cp /opt/jujuagentd $JUJU_TOOLS_DIR/jujuagentd"), tc.IsTrue)

	jujudIndex := strings.Index(script, "[ -x /usr/bin/jujud ]")
	c.Assert(jujudIndex >= 0, tc.IsTrue)
	c.Check(optAgentdIndex < jujudIndex, tc.IsTrue)
	c.Check(strings.Contains(script, "cp /usr/bin/jujud $JUJU_TOOLS_DIR/jujud"), tc.IsTrue)

	optJujudIndex := strings.Index(script, "cp /opt/jujud $JUJU_TOOLS_DIR/jujud")
	c.Assert(optJujudIndex >= 0, tc.IsTrue)
	c.Check(jujudIndex < optJujudIndex, tc.IsTrue)

	// Both binary-name compatibility branches are retained.
	c.Check(strings.Contains(script,
		"exec $JUJU_TOOLS_DIR/jujuagentd model --model-uuid=model-uuid"), tc.IsTrue)
	c.Check(strings.Contains(script,
		"exec $JUJU_TOOLS_DIR/jujud model --model-uuid=model-uuid"), tc.IsTrue)

	// Each exec command must run inside a branch that copies the binary
	// it executes. Ordering the first occurrences pins that binding
	// against the template itself: the jujuagentd exec leads in the
	// first rock branch, and the jujud exec first appears only after
	// the jujud branches open. Swapping the %[3]s and %[4]s placements
	// in the template would leave a branch executing a binary it never
	// copied, and fail this order.
	execAgentdIndex := strings.Index(script,
		"exec $JUJU_TOOLS_DIR/jujuagentd model --model-uuid=model-uuid")
	c.Assert(execAgentdIndex >= 0, tc.IsTrue)
	execJujudIndex := strings.Index(script,
		"exec $JUJU_TOOLS_DIR/jujud model --model-uuid=model-uuid")
	c.Assert(execJujudIndex >= 0, tc.IsTrue)
	c.Check(agentdIndex < execAgentdIndex, tc.IsTrue)
	c.Check(execAgentdIndex < optAgentdIndex, tc.IsTrue)
	c.Check(jujudIndex < execJujudIndex, tc.IsTrue)
	c.Check(execJujudIndex < optJujudIndex, tc.IsTrue)
}

func (s *scriptsSuite) TestAPIServerStartUpScript(c *tc.C) {
	script := fmt.Sprintf(
		caas.APIServerStartUpSh,
		"/var/lib/juju",
		"setup command",
		"pebble layer",
		"38811",
	)

	// The tool binaries are selected FHS-first, with the /opt layout of
	// the Docker image kept as a fallback.
	for _, check := range []struct {
		fhs    string
		legacy string
	}{
		{"jujud=/usr/bin/jujud", "jujud=/opt/jujud"},
		{"jujuagentd=/usr/bin/jujuagentd", "jujuagentd=/opt/jujuagentd"},
		{"pebble=/usr/lib/juju/pebble", "pebble=/opt/pebble"},
	} {
		fhsIndex := strings.Index(script, check.fhs)
		c.Assert(fhsIndex >= 0, tc.IsTrue, tc.Commentf("missing %q", check.fhs))
		legacyIndex := strings.Index(script, check.legacy)
		c.Assert(legacyIndex >= 0, tc.IsTrue, tc.Commentf("missing %q", check.legacy))
		c.Check(fhsIndex < legacyIndex, tc.IsTrue,
			tc.Commentf("%q must be selected before %q", check.fhs, check.legacy))
	}

	c.Check(strings.Contains(script, `cp "$jujud" $JUJU_TOOLS_DIR/jujud`), tc.IsTrue)
	c.Check(strings.Contains(script, `cp "$jujuagentd" $JUJU_TOOLS_DIR/jujuagentd`), tc.IsTrue)
	c.Check(strings.Contains(script, `exec "$pebble" run --http :38811 --verbose`), tc.IsTrue)

	// Juju's Pebble must not be Rockcraft's OCI entrypoint.
	c.Check(strings.Contains(script, "exec /usr/bin/pebble"), tc.IsFalse)
}

func (s *scriptsSuite) TestContainerAgentStartUpScript(c *tc.C) {
	script := caas.ContainerAgentStartUpSh

	fhsIndex := strings.Index(script, "[ -x /usr/bin/containeragent ]")
	c.Assert(fhsIndex >= 0, tc.IsTrue)
	legacyIndex := strings.Index(script, "exec /opt/containeragent")
	c.Assert(legacyIndex >= 0, tc.IsTrue)
	c.Check(fhsIndex < legacyIndex, tc.IsTrue)

	// The init arguments are passed through as positional parameters.
	c.Check(strings.Contains(script, `exec /usr/bin/containeragent "$@"`), tc.IsTrue)
	c.Check(strings.Contains(script, `exec /opt/containeragent "$@"`), tc.IsTrue)
}
