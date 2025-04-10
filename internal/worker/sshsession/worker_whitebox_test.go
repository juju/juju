package sshsession

import (
	"os"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type workerWhiteboxSuite struct {
	testing.IsolationSuite
}

var (
	_                  = gc.Suite(&workerWhiteboxSuite{})
	sshdConfigTemplate = `
# This is the sshd server system-wide configuration file.  See
# sshd_config(5) for more information.

# This sshd was compiled with PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games

# The strategy used for options in the default sshd_config shipped with
# OpenSSH is to specify options with their default value where
# possible, but leave them commented.  Uncommented options override the
# default value.

Include /etc/ssh/sshd_config.d/*.conf

Port 17023
#AddressFamily any
#ListenAddress 0.0.0.0
`
)

// TestConnectionGetterGetLocalSSHPort tests the local SSHD port can be retrieved.
// This function never actually fails, and instead defaults to 22. So we create
// a temp file with a very distinct port number to find.
func (s *workerWhiteboxSuite) TestConnectionGetterGetLocalSSHPort(c *gc.C) {
	file, err := os.CreateTemp("", "test-ssd-config")
	c.Assert(err, gc.IsNil)
	defer os.Remove(file.Name())

	_, err = file.Write([]byte(sshdConfigTemplate))
	c.Assert(err, gc.IsNil)

	l := loggo.GetLogger("test")
	cg := NewConnectionGetter(l)
	port := cg.getLocalSSHPort(file.Name())
	c.Assert(port, gc.Equals, "17023")
}
