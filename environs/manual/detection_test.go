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

	"launchpad.net/juju-core/testing"
)

type detectionSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&detectionSuite{})

// sshscript should only print the result on the first execution,
// to handle the case where it's called multiple times. On
// subsequent executions, it should find the next 'ssh' in $PATH
// and exec that.
var sshscript = `#!/bin/bash
if [ ! -e "$0.run" ]; then
    touch "$0.run"
    cat /dev/stdin>/dev/null
%s
    exit %d
else
    export PATH=${PATH#*:}
    exec ssh $*
fi`

// sshresponse creates a fake "ssh" command in a new $PATH,
// updates $PATH, and returns a function to reset $PATH to
// its original value when called.
func sshresponse(c *gc.C, output string, rc int) func() {
	fakebin := c.MkDir()
	ssh := filepath.Join(fakebin, "ssh")
	if output != "" {
		output = fmt.Sprintf("cat<<EOF\n%s\nEOF", output)
	}
	script := fmt.Sprintf(sshscript, output, rc)
	err := ioutil.WriteFile(ssh, []byte(script), 0777)
	c.Assert(err, gc.IsNil)
	return testing.PatchEnvironment("PATH", fakebin+":"+os.Getenv("PATH"))
}

func (s *detectionSuite) TestDetectSeries(c *gc.C) {
	response := strings.Join([]string{
		"edgy",
		"armv4",
		"MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer sshresponse(c, response, 0)()
	_, series, err := detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(series, gc.Equals, "edgy")
}

func (s *detectionSuite) TestDetectionError(c *gc.C) {
	defer sshresponse(c, "oh noes", 33)()
	_, _, err := detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.ErrorMatches, "exit status 33 \\(oh noes\\)")
}

func (s *detectionSuite) TestDetectHardwareCharacteristics(c *gc.C) {
	// Single CPU socket, single core, no hyper-threading.
	response := strings.Join([]string{
		"edgy", "armv4", "MemTotal: 4096 kB",
		"processor: 0",
	}, "\n")
	defer sshresponse(c, response, 0)()
	hc, _, err := detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(hc.String(), gc.Equals, "arch=arm cpu-cores=1 mem=4M")

	// Single CPU socket, single core, hyper-threading.
	response = strings.Join([]string{
		"edgy", "armv4", "MemTotal: 4096 kB",
		"processor: 0",
		"physical id: 0",
		"cpu cores: 1",
		"processor: 1",
		"physical id: 0",
		"cpu cores: 1",
	}, "\n")
	defer sshresponse(c, response, 0)()
	hc, _, err = detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(hc.String(), gc.Equals, "arch=arm cpu-cores=1 mem=4M")

	// Single CPU socket, dual-core, no hyper-threading.
	response = strings.Join([]string{
		"edgy", "armv4", "MemTotal: 4096 kB",
		"processor: 0",
		"physical id: 0",
		"cpu cores: 2",
		"processor: 1",
		"physical id: 0",
		"cpu cores: 2",
	}, "\n")
	defer sshresponse(c, response, 0)()
	hc, _, err = detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(hc.String(), gc.Equals, "arch=arm cpu-cores=2 mem=4M")

	// Dual CPU socket, each single-core, hyper-threading.
	response = strings.Join([]string{
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
	}, "\n")
	defer sshresponse(c, response, 0)()
	hc, _, err = detectSeriesAndHardwareCharacteristics("whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(hc.String(), gc.Equals, "arch=arm cpu-cores=2 mem=4M")
}
