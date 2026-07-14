// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"testing"

	"github.com/juju/tc"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/cmd/jujuagentd/util"
)

func TestIsControllerFlagManifold(t *testing.T) {
	tc.Run(t, &stateFlagSuite{})
}

type stateFlagSuite struct{}

func (s *stateFlagSuite) TestControllerAndNonControllerFlags(c *tc.C) {
	for _, test := range []struct {
		name           string
		isController   bool
		controllerFlag bool
	}{
		{name: "controller", isController: true, controllerFlag: true},
		{name: "non-controller", controllerFlag: false},
	} {
		c.Logf("testing %s", test.name)
		func() {
			getter := dt.StubGetter(map[string]any{"state-config-watcher": test.isController})
			assertFlag := func(yes, want bool) {
				manifold := util.IsControllerFlagManifold("state-config-watcher", yes)
				worker, err := manifold.Start(c.Context(), getter)
				c.Assert(err, tc.ErrorIsNil)
				defer workertest.DirtyKill(c, worker)

				var flag engine.Flag
				err = manifold.Output(worker, &flag)
				c.Assert(err, tc.ErrorIsNil)
				c.Check(flag.Check(), tc.Equals, want)
			}

			assertFlag(true, test.controllerFlag)
			assertFlag(false, !test.controllerFlag)
		}()
	}
}
