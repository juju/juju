package local

import (
	"encoding/xml"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// networks represents a network with the given name, bridge, ip and subnet informations.
type network struct {
	XMLName xml.Name `xml:"network"`
	Name    string   `xml:"name"`
	Bridge  bridge
	Ip      ip
	Subnet  int
}

// ip represents an ip with the given address and netmask.
type ip struct {
	XMLName xml.Name `xml:"ip"`
	Address string   `xml:"address,attr"`
	Netmask string   `xml:"netmask,attr"`
}

// bridge represents a briddge with the given name.
type bridge struct {
	XMLName xml.Name `xml:"bridge"`
	Name    string   `xml:"name,attr"`
}

// loadAttributes loads the attributes for a network.
// It's use virsh net-dumpxml to get network info.
func (n *network) loadAttributes() error {
	output, err := exec.Command("virsh", "net-dumpxml", n.Name).Output()
	if err != nil {
		return err
	}
	xml.Unmarshal(output, &n)
	return nil
}

// running returns true if the network name is in the
// list of networks and is active.
func (n *network) running() bool {
	networks, err := listNetworks()
	if err != nil {
		return false
	}
	return networks[n.Name]
}

// exists returns true if the network name is in the
// list of networks.
func (n *network) exists() bool {
	networks, err := listNetworks()
	if err != nil {
		return false
	}
	check, _ := networks[n.Name]
	return check
}

const libVirtNetworkTemplate = `
<network>
  <name>{{.Name}}</name>
  <bridge name='vbr-{{.Name}}-%d' />
  <forward/>
  <ip address='192.168.{{.Subnet}}.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='192.168.{{.Subnet}}.2' end='192.168.{{.Subnet}}.254' />
    </dhcp>
  </ip>
</network>
`

// start starts a network if the networks isn't already active.
// It's use virsh net-start to start the network.
// If the network does not exists, the network is created using
// virsh net-define.
func (n *network) start() error {
	if n.exists() {
		if n.running() {
			return nil
		} else {
			return exec.Command("virsh", "net-start", n.Name).Run()
		}
	}
	file, err := ioutil.TempFile(os.TempDir(), "network")
	if err != nil {
		return err
	}
	tmpl, err := template.New("network").Parse(libVirtNetworkTemplate)
	if err != nil {
		return err
	}
	err = tmpl.Execute(file, n)
	if err != nil {
		return err
	}
	file.Close()
	err = exec.Command("virsh", "net-define", file.Name()).Run()
	if err != nil {
		return err
	}
	return exec.Command("virsh", "net-start", n.Name).Run()
}

// destroy destroys a network.
func (n *network) destroy() error {
	return exec.Command("virsh", "net-undefine", n.Name).Run()
}

// listNetworks Returns a map[string]bool of network name to active status.
func listNetworks() (map[string]bool, error) {
	networks := map[string]bool{}
	output, err := exec.Command("virsh", "net-list", "--all").Output()
	if err != nil {
		return networks, nil
	}
	// Take the header off
	lines := strings.Split(string(output), "\n")[2:]
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		networks[fields[0]] = fields[1] == "active"
	}
	return networks, nil
}
