// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type detectionSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&detectionSuite{})

// sshscript should only print the result on the first execution,
// to handle the case where it's called multiple times. On
// subsequent executions, it should find the next 'ssh' in $PATH
// and exec that.
var sshscript = `#!/bin/bash
if [ ! -e "$0.run" ]; then
    touch "$0.run"
    diff "$0.expected-input" -
    exitcode=$?
    if [ $exitcode -ne 0 ]; then
        echo "ERROR: did not match expected input" >&2
        exit $exitcode
    fi
    # stdout
    %s
    # stderr
    %s
    exit %d
else
    export PATH=${PATH#*:}
    exec ssh $*
fi`

// sshresponse creates a fake "ssh" command in a new $PATH,
// updates $PATH, and returns a function to reset $PATH to
// its original value when called.
//
// output may be:
//    - nil (no output)
//    - a string (stdout)
//    - a slice of strings, of length two (stdout, stderr)
func sshresponse(c *gc.C, input string, output interface{}, rc int) func() {
	fakebin := c.MkDir()
	ssh := filepath.Join(fakebin, "ssh")
	sshexpectedinput := ssh + ".expected-input"
	var stdout, stderr string
	switch output := output.(type) {
	case nil:
	case string:
		stdout = fmt.Sprintf("cat<<EOF\n%s\nEOF", output)
	case []string:
		stdout = fmt.Sprintf("cat<<EOF\n%s\nEOF", output[0])
		stderr = fmt.Sprintf("cat>&2<<EOF\n%s\nEOF", output[1])
	}
	script := fmt.Sprintf(sshscript, stdout, stderr, rc)
	err := ioutil.WriteFile(ssh, []byte(script), 0777)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(sshexpectedinput, []byte(input), 0644)
	c.Assert(err, gc.IsNil)
	return testbase.PatchEnvironment("PATH", fakebin+":"+os.Getenv("PATH"))
}

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
	defer sshresponse(c, checkProvisionedScript, "", 0)()
	provisioned, err := checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	defer sshresponse(c, checkProvisionedScript, "non-empty", 0)()
	provisioned, err = checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsTrue)

	// stderr should not affect result.
	defer sshresponse(c, checkProvisionedScript, []string{"", "non-empty-stderr"}, 0)()
	provisioned, err = checkProvisioned("example.com")
	c.Assert(err, gc.IsNil)
	c.Assert(provisioned, jc.IsFalse)

	// if the script fails for whatever reason, then checkProvisioned
	// will return an error. stderr will be included in the error message.
	defer sshresponse(c, checkProvisionedScript, []string{"non-empty-stdout", "non-empty-stderr"}, 255)()
	_, err = checkProvisioned("example.com")
	c.Assert(err, gc.ErrorMatches, "exit status 255 \\(non-empty-stderr\\)")
}
