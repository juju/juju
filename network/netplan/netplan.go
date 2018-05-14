package netplan

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	goyaml "gopkg.in/yaml.v2"
)

// Representation of netplan YAML format as Go structures
// The order of fields is consistent with Netplan docs
type Nameservers struct {
	Search    []string `yaml:"search,omitempty,flow"`
	Addresses []string `yaml:"addresses,omitempty,flow"`
}

// Interface includes all the fields that are common between all interfaces (ethernet, wifi, bridge, bond)
type Interface struct {
	AcceptRA  *bool    `yaml:"accept-ra,omitempty"`
	Addresses []string `yaml:"addresses,omitempty"`
	// Critical doesn't have to be *bool because it is only used if True
	Critical bool `yaml:"critical,omitempty"`
	// DHCP4 defaults to true, so we must use a pointer to know if it was specified as false
	DHCP4          *bool         `yaml:"dhcp4,omitempty"`
	DHCP6          *bool         `yaml:"dhcp6,omitempty"`
	DHCPIdentifier string        `yaml:"dhcp-identifier,omitempty"` // "duid" or  "mac"
	Gateway4       string        `yaml:"gateway4,omitempty"`
	Gateway6       string        `yaml:"gateway6,omitempty"`
	Nameservers    Nameservers   `yaml:"nameservers,omitempty"`
	MACAddress     string        `yaml:"macaddress,omitempty"`
	MTU            int           `yaml:"mtu,omitempty"`
	Renderer       string        `yaml:"renderer,omitempty"` // NetworkManager or networkd
	Routes         []Route       `yaml:"routes,omitempty"`
	RoutingPolicy  []RoutePolicy `yaml:"routing-policy,omitempty"`
	// Optional doesn't have to be *bool because it is only used if True
	Optional bool `yaml:"optional,omitempty"`
}

// Ethernet defines fields for just Ethernet devices
type Ethernet struct {
	Match     map[string]string `yaml:"match,omitempty"`
	Wakeonlan bool              `yaml:"wakeonlan,omitempty"`
	SetName   string            `yaml:"set-name,omitempty"`
	Interface `yaml:",inline"`
}
type AccessPoint struct {
	Password string `yaml:"password,omitempty"`
	Mode     string `yaml:"mode,omitempty"`
	Channel  int    `yaml:"channel,omitempty"`
}
type Wifi struct {
	Match        map[string]string      `yaml:"match,omitempty"`
	SetName      string                 `yaml:"set-name,omitempty"`
	Wakeonlan    bool                   `yaml:"wakeonlan,omitempty"`
	AccessPoints map[string]AccessPoint `yaml:"access-points,omitempty"`
	Interface    `yaml:",inline"`
}

type Bridge struct {
	Interfaces []string `yaml:"interfaces,omitempty,flow"`
	Interface  `yaml:",inline"`
	// TODO: parameters
}

type Route struct {
	From   string `yaml:"from,omitempty"`
	OnLink *bool  `yaml:"on-link,omitempty"`
	Scope  string `yaml:"scope,omitempty"`
	Table  *int   `yaml:"table,omitempty"`
	To     string `yaml:"to,omitempty"`
	Type   string `yaml:"type,omitempty"`
	Via    string `yaml:"via,omitempty"`
	Metric *int   `yaml:"metric,omitempty"`
}

type RoutePolicy struct {
	From          string `yaml:"from,omitempty"`
	Mark          *int   `yaml:"mark,omitempty"`
	Priority      *int   `yaml:"priority,omitempty"`
	Table         *int   `yaml:"table,omitempty"`
	To            string `yaml:"to,omitempty"`
	TypeOfService *int   `yaml:"type-of-service,omitempty"`
}

type Network struct {
	Version   int                 `yaml:"version"`
	Renderer  string              `yaml:"renderer,omitempty"`
	Ethernets map[string]Ethernet `yaml:"ethernets,omitempty"`
	Wifis     map[string]Wifi     `yaml:"wifis,omitempty"`
	Bridges   map[string]Bridge   `yaml:"bridges,omitempty"`
	Bonds     map[string]Bond     `yaml:"bonds,omitempty"`
	VLANs     map[string]VLAN     `yaml:"vlans,omitempty"`
	Routes    []Route             `yaml:"routes,omitempty"`
}

type Netplan struct {
	Network         Network `yaml:"network"`
	sourceDirectory string
	sourceFiles     []string
	backedFiles     map[string]string
	writtenFile     string
}

// VLAN represents the structures for defining VLAN sections
type VLAN struct {
	Id        *int   `yaml:"id,omitempty"`
	Link      string `yaml:"link,omitempty"`
	Interface `yaml:",inline"`
}

// Bond is the interface definition of the bonds: section of netplan
type Bond struct {
	Interfaces []string `yaml:"interfaces,omitempty,flow"`
	Interface  `yaml:",inline"`
	Parameters BondParameters `yaml:"parameters,omitempty"`
}

// IntString is used to specialize values that can be integers or strings
type IntString struct {
	Int    *int
	String *string
}

func (i *IntString) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var asInt int
	var err error
	if err = unmarshal(&asInt); err == nil {
		i.Int = &asInt
		return nil
	}
	var asString string
	if err = unmarshal(&asString); err == nil {
		i.String = &asString
		return nil
	}
	return errors.Annotatef(err, "not valid as an int or a string")
}

func (i IntString) MarshalYAML() (interface{}, error) {
	if i.Int != nil {
		return *i.Int, nil
	} else if i.String != nil {
		return *i.String, nil
	}
	return nil, nil
}

// For a definition of what netplan supports see here:
// https://github.com/CanonicalLtd/netplan/blob/7afef6af053794a400d96f89a81c938c08420783/src/parse.c#L1180
// For a definition of what the parameters mean or what values they can contain, see here:
// https://www.kernel.org/doc/Documentation/networking/bonding.txt
// Note that most parameters can be specified as integers or as strings, which you need to be careful with YAML
// as it defaults to strongly typing them.
// TODO: (jam 2018-05-14) Should we be sorting the attributes alphabetically?
type BondParameters struct {
	Mode               IntString `yaml:"mode,omitempty"`
	LACPRate           IntString `yaml:"lacp-rate,omitempty"`
	MIIMonitorInterval *int      `yaml:"mii-monitor-interval,omitempty"`
	MinLinks           *int      `yaml:"min-links,omitempty"`
	TransmitHashPolicy string    `yaml:"transmit-hash-policy,omitempty"`
	ADSelect           IntString `yaml:"ad-select,omitempty"`
	AllSlavesActive    bool      `yaml:"all-slaves-active,omitempty"`
	ARPInterval        *int      `yaml:"arp-interval,omitempty"`
	ARPIPTargets       []string  `yaml:"arp-ip-targets,omitempty"`
	ARPValidate        IntString `yaml:"arp-validate,omitempty"`
	ARPAllTargets      IntString `yaml:"arp-all-targets,omitempty"`
	UpDelay            *int      `yaml:"up-delay,omitempty"`
	DownDelay          *int      `yaml:"down-delay,omitempty"`
	FailOverMACPolicy  IntString `yaml:"fail-over-mac-policy,omitempty"`
	// Netplan misspelled this as 'gratuitious-arp', not sure if it works with that name.
	// We may need custom handling of both spellings.
	GratuitousARP         *int      `yaml:"gratuitious-arp,omitempty"`
	PacketsPerSlave       *int      `yaml:"packets-per-slave,omitempty"`
	PrimaryReselectPolicy IntString `yaml:"primary-reselect-policy,omitempty"`
	ResendIGMP            *int      `yaml:"resend-igmp,omitempty"`
	// bonding.txt says that this can be a value from 1-0x7fffffff, should we be forcing it to be a hex value?
	LearnPacketInterval *int   `yaml:"learn-packet-interval,omitempty"`
	Primary             string `yaml:"primary,omitempty"`
}

// BridgeDeviceById takes a deviceId and creates a bridge with this device
// using this devices config
func (np *Netplan) BridgeEthernetById(deviceId string, bridgeName string) (err error) {
	ethernet, ok := np.Network.Ethernets[deviceId]
	if !ok {
		return errors.NotFoundf("Device with id %q for bridge %q", deviceId, bridgeName)
	}
	for bName, bridge := range np.Network.Bridges {
		for _, i := range bridge.Interfaces {
			if i == deviceId {
				// The device is already properly bridged, we're not doing anything
				if bridgeName == bName {
					return nil
				} else {
					return errors.AlreadyExistsf("Device %q is already bridged in bridge %q instead of %q", deviceId, bName, bridgeName)
				}
			}
		}
		if bridgeName == bName {
			return errors.AlreadyExistsf("Cannot bridge device %q on bridge %q - bridge named %q", deviceId, bridgeName, bridgeName)
		}
	}
	// copy aside and clear the IP settings from the original Ethernet device, except for MTU
	intf := ethernet.Interface
	ethernet.Interface = Interface{MTU: intf.MTU}
	// create a bridge
	if np.Network.Bridges == nil {
		np.Network.Bridges = make(map[string]Bridge)
	}
	np.Network.Bridges[bridgeName] = Bridge{
		Interfaces: []string{deviceId},
		Interface:  intf,
	}
	np.Network.Ethernets[deviceId] = ethernet
	return nil
}

func Unmarshal(in []byte, out interface{}) (err error) {
	return goyaml.UnmarshalStrict(in, out)
}

func Marshal(in interface{}) (out []byte, err error) {
	return goyaml.Marshal(in)
}

// readYamlFile reads netplan yaml into existing netplan structure
// TODO(wpk) 2017-06-14 When reading files sequentially netplan replaces single
// keys with new values, we have to simulate this behaviour.
// https://bugs.launchpad.net/juju/+bug/1701429
func (np *Netplan) readYamlFile(path string) (err error) {
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = Unmarshal(contents, np)
	if err != nil {
		return err
	}

	return nil
}

type sortableFileInfos []os.FileInfo

func (fil sortableFileInfos) Len() int {
	return len(fil)
}

func (fil sortableFileInfos) Less(i, j int) bool {
	return fil[i].Name() < fil[j].Name()
}

func (fil sortableFileInfos) Swap(i, j int) {
	fil[i], fil[j] = fil[j], fil[i]
}

// ReadDirectory reads the contents of a netplan directory and
// returns complete config.
func ReadDirectory(dirPath string) (np Netplan, err error) {
	fileInfos, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return np, err
	}
	np.sourceDirectory = dirPath
	sortedFileInfos := sortableFileInfos(fileInfos)
	sort.Sort(sortedFileInfos)
	for _, fileInfo := range sortedFileInfos {
		if !fileInfo.IsDir() && strings.HasSuffix(fileInfo.Name(), ".yaml") {
			np.sourceFiles = append(np.sourceFiles, fileInfo.Name())
		}
	}
	for _, fileName := range np.sourceFiles {
		err := np.readYamlFile(path.Join(np.sourceDirectory, fileName))
		if err != nil {
			return np, err
		}
	}
	return np, nil
}

// MoveYamlsToBak moves source .yaml files in a directory to .yaml.bak.(timestamp), except
func (np *Netplan) MoveYamlsToBak() (err error) {
	if np.backedFiles != nil {
		return errors.Errorf("Cannot backup netplan yamls twice")
	}
	suffix := fmt.Sprintf(".bak.%d", time.Now().Unix())
	np.backedFiles = make(map[string]string)
	for _, file := range np.sourceFiles {
		newFilename := fmt.Sprintf("%s%s", file, suffix)
		oldFile := path.Join(np.sourceDirectory, file)
		newFile := path.Join(np.sourceDirectory, newFilename)
		err = os.Rename(oldFile, newFile)
		if err != nil {
			logger.Errorf("Cannot rename %s to %s - %q", oldFile, newFile, err.Error())
		}
		np.backedFiles[oldFile] = newFile
	}
	return nil
}

// Write writes merged netplan yaml to file specified by path. If path is empty filename is autogenerated
func (np *Netplan) Write(inPath string) (filePath string, err error) {
	if np.writtenFile != "" {
		return "", errors.Errorf("Cannot write the same netplan twice")
	}
	if inPath == "" {
		i := 99
		for ; i > 0; i-- {
			filePath = path.Join(np.sourceDirectory, fmt.Sprintf("%0.2d-juju.yaml", i))
			_, err = os.Stat(filePath)
			if os.IsNotExist(err) {
				break
			}
		}
		if i == 0 {
			return "", errors.Errorf("Can't generate a filename for netplan YAML")
		}
	} else {
		filePath = inPath
	}
	tmpFilePath := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	out, err := Marshal(np)
	if err != nil {
		return "", err
	}
	err = ioutil.WriteFile(tmpFilePath, out, 0644)
	if err != nil {
		return "", err
	}
	err = os.Rename(tmpFilePath, filePath)
	if err != nil {
		return "", err
	}
	np.writtenFile = filePath
	return filePath, nil
}

// Rollback moves backed up files to original locations and removes written file
func (np *Netplan) Rollback() (err error) {
	if np.writtenFile != "" {
		os.Remove(np.writtenFile)
	}
	for oldFile, newFile := range np.backedFiles {
		err = os.Rename(newFile, oldFile)
		if err != nil {
			logger.Errorf("Cannot rename %s to %s - %q", newFile, oldFile, err.Error())
		}
	}
	np.backedFiles = nil
	np.writtenFile = ""
	return nil
}

func (np *Netplan) FindEthernetByMAC(mac string) (device string, err error) {
	for id, ethernet := range np.Network.Ethernets {
		if v, ok := ethernet.Match["macaddress"]; ok && v == mac {
			return id, nil
		}
		if ethernet.MACAddress == mac {
			return id, nil
		}
	}
	return "", errors.NotFoundf("Ethernet device with MAC %q", mac)
}

func (np *Netplan) FindEthernetByName(name string) (device string, err error) {
	for id, ethernet := range np.Network.Ethernets {
		if matchName, ok := ethernet.Match["name"]; ok {
			// Netplan uses simple wildcards for name matching - so does filepath.Match
			if match, err := filepath.Match(matchName, name); err == nil && match {
				return id, nil
			}
		}
		if ethernet.SetName == name {
			return id, nil
		}
	}
	if _, ok := np.Network.Ethernets[name]; ok {
		return name, nil
	}
	return "", errors.NotFoundf("Ethernet device with name %q", name)
}

func (np *Netplan) FindVLANByMAC(mac string) (device string, err error) {
	for id, vlan := range np.Network.VLANs {
		if vlan.MACAddress == mac {
			return id, nil
		}
	}
	return "", errors.NotFoundf("VLAN device with MAC %q", mac)
}

func (np *Netplan) FindVLANByName(name string) (device string, err error) {
	if _, ok := np.Network.VLANs[name]; ok {
		return name, nil
	}
	return "", errors.NotFoundf("VLAN device with name %q", name)
}

func (np *Netplan) FindBondByMAC(mac string) (device string, err error) {
	for id, bonds := range np.Network.Bonds {
		if bonds.MACAddress == mac {
			return id, nil
		}
	}
	return "", errors.NotFoundf("Bond device with MAC %q", mac)
}

func (np *Netplan) FindBondByName(name string) (device string, err error) {
	if _, ok := np.Network.Bonds[name]; ok {
		return name, nil
	}
	return "", errors.NotFoundf("Bond device with name %q", name)
}

type DeviceType string

const (
	TypeEthernet = DeviceType("ethernet")
	TypeVLAN     = DeviceType("vlan")
	TypeBond     = DeviceType("bond")
)

// FindDeviceByMACOrName will look for an Ethernet, VLAN or Bond matching the MAC address or the Name of the device.
// If MAC Address is supplied we check for matches there first, and then fall back to by name.
func (np *Netplan) FindDeviceByMACOrName(mac, name string) (string, DeviceType, error) {
	if mac != "" {
		bond, err := np.FindBondByMAC(mac)
		if err == nil {
			return bond, TypeBond, nil
		}
		if !errors.IsNotFound(err) {
			return "", "", errors.Trace(err)
		}
		vlan, err := np.FindVLANByMAC(mac)
		if err == nil {
			return vlan, TypeVLAN, nil
		}
		ethernet, err := np.FindEthernetByMAC(mac)
		if err == nil {
			return ethernet, TypeEthernet, nil
		}
	}
	if name != "" {
		bond, err := np.FindBondByName(name)
		if err == nil {
			return bond, TypeBond, nil
		}
		if !errors.IsNotFound(err) {
			return "", "", errors.Trace(err)
		}
		vlan, err := np.FindVLANByName(name)
		if err == nil {
			return vlan, TypeVLAN, nil
		}
		ethernet, err := np.FindEthernetByName(name)
		if err == nil {
			return ethernet, TypeEthernet, nil
		}

	}
	return "", "", errors.NotFoundf("device - name %q MAC %q", name, mac)
}
