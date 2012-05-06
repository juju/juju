package local

import (
	. "launchpad.net/gocheck"
    "os"
)

var scriptText = `#!/bin/bash
#!/bin/bash

case "$1" in
net-list)
echo "Name                 State      Autostart"
echo "-----------------------------------------"
echo "default              active     yes"
;;

net-dumpxml)
echo "<network>"
echo "<name>default</name>"
echo "<uuid>7f5d45e4-2fa2-f713-0229-fb1fea419e3b</uuid>"
echo "<forward mode='nat'/>"
echo "<bridge name='virbr0' stp='on' delay='0' />"
echo "<ip address='192.168.122.1' netmask='255.255.255.0'>"
echo "<dhcp>"
echo "<range start='192.168.122.2' end='192.168.122.254' />"
echo "</dhcp>"
echo "</ip>"
echo "</network>"
;;

net-start)
echo "net-start"
;;

net-define)
echo "net-define"
;;

esac

exit 0
`

type networkSuite struct{}

var _ = Suite(&networkSuite{})
var oldPath string

func (s *networkSuite) SetUpSuite(c *C) {
    oldPath = os.Getenv("PATH")
    dir := c.MkDir()
    os.Setenv("PATH", dir+":"+oldPath)
    writeScript(c, dir)
}

func (s *networkSuite) TearDownSuite(c *C) {
    os.Setenv("PATH", oldPath)
}

func (s *networkSuite) TestStartNetwork(c *C) {
    //start a newtork that already exists
    n := network{Name: "default"}
    err := n.start()
    c.Assert(err, IsNil)

    //start a new network
    n = network{Name: "newnet"}
    err = n.start()
    c.Assert(err, IsNil)
}

func (s *networkSuite) TestLoadAttributes(c *C) {
    n := network{Name: "default"}
    err := n.loadAttributes()
    c.Assert(err, IsNil)
    c.Assert(n.Name, Equals, "default")
    c.Assert(n.Bridge.Name, Equals, "virbr0")
    c.Assert(n.Ip.Ip, Equals, "192.168.122.1")
    c.Assert(n.Ip.Mask, Equals, "255.255.255.0")
}

func (s *networkSuite) TestRunning(c *C) {
    n := network{Name: "default"}
    running, err := n.running()
    c.Assert(err, IsNil)
    c.Assert(running, Equals, true)

    n = network{Name: "fakeName"}
    running, err = n.running()
    c.Assert(err, IsNil)
    c.Assert(running, Equals, false)
}

func (s *networkSuite) TestNetworkExists(c *C) {
    n := network{Name: "default"}
    exists, err := n.exists()
    c.Assert(err, IsNil)
    c.Assert(exists, Equals, true)

    n = network{Name: "fakeName"}
    exists, err = n.exists()
    c.Assert(err, IsNil)
    c.Assert(exists, Equals, false)
}

func (s *networkSuite) TestListNetworks(c *C) {
    expected := map[string]bool{"default": true}
    networks, err := listNetworks()
    c.Assert(err, IsNil)
    c.Assert(networks, DeepEquals, expected)
}

func writeScript(c *C, dir string) {
    f, err := os.OpenFile(dir+"/virsh", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0777)
    c.Assert(err, IsNil)
    defer f.Close()
    _, err = f.WriteString(scriptText)
    c.Assert(err, IsNil)
}
