package local

import (
	"encoding/xml"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// network represents a local virtual network.
type network struct {
	XMLName xml.Name `xml:"network"`
	Name    string   `xml:"name"`
	Bridge  bridge   `xml:"bridge"`
	Ip      ip       `xml:"ip"`
	Subnet  int
}

// ip represents an ip with the given address and netmask.
type ip struct {
	Ip   string `xml:"address,attr"`
	Mask string `xml:"netmask,attr"`
}

// bridge represents a briddge with the given name.
type bridge struct {
	Name string `xml:"name,attr"`
}

// newNetwork returns a started network.
func newNetwork(name string, subnet int) (*network, error) {
	n := network{Name: name, Subnet: subnet}
	err := n.start()
	if err != nil {
		return nil, err
	}
	err = n.loadAttributes()
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// loadAttributes loads the attributes for a network.
func (n *network) loadAttributes() error {
	output, err := exec.Command("virsh", "net-dumpxml", n.Name).Output()
	if err != nil {
		return err
	}
	return xml.Unmarshal(output, &n)
}

// running returns true if network name is in the
// list of networks and is active.
func (n *network) running() (bool, error) {
	networks, err := listNetworks()
	if err != nil {
		return false, err
	}
	return networks[n.Name], nil
}

// exists returns true if network name is in the
// list of networks.
func (n *network) exists() (bool, error) {
	networks, err := listNetworks()
	if err != nil {
		return false, err
	}
	_, exists := networks[n.Name]
	return exists, nil
}

// virsh replaces %d with an auto increment
// number to make the bridge name unique
var libVirtNetworkTemplate = template.Must(template.New("").Parse(`
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
`))

// start ensures that the network is started.
// If the network name does not exist, it is created.
func (n *network) start() error {
	exists, err := n.exists()
	if err != nil {
		return err
	}
	if exists {
		running, err := n.running()
		if err != nil {
			return err
		}
		if running {
			return nil
		}
		return exec.Command("virsh", "net-start", n.Name).Run()
	}
	file, err := ioutil.TempFile(os.TempDir(), "network")
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	err = libVirtNetworkTemplate.Execute(file, n)
	if err != nil {
		return err
	}
	err = exec.Command("virsh", "net-define", file.Name()).Run()
	if err != nil {
		return err
	}
	return exec.Command("virsh", "net-start", n.Name).Run()
}

// listNetworks returns a map from network name to active status.
func listNetworks() (map[string]bool, error) {
	output, err := exec.Command("virsh", "net-list", "--all").Output()
	if err != nil {
		return nil, err
	}
	// Remove the header.
	networks := map[string]bool{}
	lines := strings.Split(string(output), "\n")
	if len(lines) < 3 {
		return networks, nil
	}
	lines = lines[2:]
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 2 {
			networks[fields[0]] = fields[1] == "active"
		}
	}
	return networks, nil
}
