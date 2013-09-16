// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type detectionSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&detectionSuite{})

func (s *detectionSuite) TestDetectSeries(c *gc.C) {
	response := strings.Join([]string{
		"edgy",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer sshresponse(c, detectionScript, response, 0)()
	_, series, err := detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.Equals, "edgy")
}

func (s *detectionSuite) TestDetectionError(c *gc.C) {
	defer sshresponse(c, detectionScript, "oh noes", 33)()
	_, _, err := detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.ErrorMatches, "exit status 33 \\(oh noes\\)")
}

func (s *detectionSuite) TestDetectHardwareCharacteristics(c *gc.C) {
	tests := []struct {
		summary        string
		scriptResponse []string
		expectedHc     string
	}{{
		"Single CPU socket, single core, no hyper-threading",
		[]string{"edgy", "armv4", "MemTotal: 4096 kB", "processor: 0"},
		"arch=arm cpu-cores=1 mem=4M",
	}, {
		"Single CPU socket, single core, hyper-threading",
		[]string{
			"edgy", "armv4", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 1",
		},
		"arch=arm cpu-cores=1 mem=4M",
	}, {
		"Single CPU socket, dual-core, no hyper-threading",
		[]string{
			"edgy", "armv4", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 2",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 2",
		},
		"arch=arm cpu-cores=2 mem=4M",
	}, {
		"Dual CPU socket, each single-core, hyper-threading",
		[]string{
			"edgy", "armv4", "MemTotal: 4096 kB",
			"processor: 0",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 1",
			"physical id: 0",
			"cpu cores: 1",
			"processor: 2",
			"physical id: 1",
			"cpu cores: 1",
			"processor: 3",
			"physical id: 1",
			"cpu cores: 1",
		},
		"arch=arm cpu-cores=2 mem=4M",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.summary)
		scriptResponse := strings.Join(test.scriptResponse, "\n")
		defer sshresponse(c, detectionScript, scriptResponse, 0)()
		hc, _, err := detectSeriesAndHardwareCharacteristics("hostname")
		c.Assert(err, gc.IsNil)
		c.Assert(hc.String(), gc.Equals, test.expectedHc)
	}
}

func (s *detectionSuite) TestCheckProvisioned(c *gc.C) {
	defer sshresponse(c, "", "", 0)()
	provisioned, err := checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	defer sshresponse(c, "", "non-empty", 0)()
	provisioned, err = checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsTrue)

	// stderr should not affect result.
	defer sshresponse(c, "", []string{"", "non-empty-stderr"}, 0)()
	provisioned, err = checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer sshresponse(c, "", []string{"non-empty-stdout", "non-empty-stderr"}, 255)()
	_, err = checkProvisioned("example.com")
	c.Assert(err, gc.ErrorMatches, "exit status 255 \\(non-empty-stderr\\)")
}
