// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"testing"

	"github.com/juju/tc"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/agent/engine"
)

func TestNotPrimaryControllerFlagManifold(c *testing.T) {
	tc.Run(c, &notPrimaryControllerFlagManifoldSuite{})
}

type notPrimaryControllerFlagManifoldSuite struct{}

func (*notPrimaryControllerFlagManifoldSuite) TestOutputsInverseFlag(c *tc.C) {
	for _, test := range []struct {
		name     string
		input    bool
		expected bool
	}{
		{name: "source true", input: true, expected: false},
		{name: "source false", input: false, expected: true},
	} {
		source := engine.NewStaticFlagWorker(test.input)
		manifold := NotPrimaryControllerFlagManifold("source")
		worker, err := manifold.Start(c.Context(), dependencytesting.StubGetter(map[string]any{
			"source": source,
		}))
		c.Assert(err, tc.ErrorIsNil)

		var flag engine.Flag
		c.Assert(manifold.Output(worker, &flag), tc.ErrorIsNil)
		c.Check(flag.Check(), tc.Equals, test.expected, tc.Commentf("%s", test.name))

		workertest.CleanKill(c, worker)
		workertest.CleanKill(c, source)
	}
}
