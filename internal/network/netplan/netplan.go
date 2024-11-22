// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package netplan

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
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

	// Configure the link-local addresses to bring up. Valid options are
	// "ipv4" and "ipv6". According to the netplan reference, netplan will
	// only bring up ipv6 addresses if *no* link-local attribute is
	// specified. On the other hand, if an empty link-local attribute is
	// specified, this instructs netplan not to bring any ipv4/ipv6 address
	// up.
	LinkLocal *[]string `yaml:"link-local,omitempty"`

	// According to the netplan examples, this section typically includes
	// some OVS-specific configuration bits. However, MAAS may just
	// include an empty block to indicate the presence of an OVS-managed
	// bridge (LP1942328). As a workaround, we make this an optional map
	// so we can tell whether it is present (but empty) vs not being
	// present.
	//
	// See: https://github.com/canonical/netplan/blob/main/examples/openvswitch.yaml
	OVSParameters *map[string]interface{} `yaml:"openvswitch,omitempty"`
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

type BridgeParameters struct {
	AgeingTime   *int           `yaml:"ageing-time,omitempty"`
	ForwardDelay IntString      `yaml:"forward-delay,omitempty"`
	HelloTime    *int           `yaml:"hello-time,omitempty"`
	MaxAge       *int           `yaml:"max-age,omitempty"`
	PathCost     map[string]int `yaml:"path-cost,omitempty"`
	PortPriority map[string]int `yaml:"port-priority,omitempty"`
	Priority     *int           `yaml:"priority,omitempty"`
	STP          *bool          `yaml:"stp,omitempty"`
}

type Bridge struct {
	Interfaces []string `yaml:"interfaces,omitempty,flow"`
	Interface  `yaml:",inline"`
	Parameters BridgeParameters `yaml:"parameters,omitempty"`
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
	MIIMonitorInterval IntString `yaml:"mii-monitor-interval,omitempty"`
	MinLinks           *int      `yaml:"min-links,omitempty"`
	TransmitHashPolicy string    `yaml:"transmit-hash-policy,omitempty"`
	ADSelect           IntString `yaml:"ad-select,omitempty"`
	AllSlavesActive    *bool     `yaml:"all-slaves-active,omitempty"`
	ARPInterval        *int      `yaml:"arp-interval,omitempty"`
	ARPIPTargets       []string  `yaml:"arp-ip-targets,omitempty"`
	ARPValidate        IntString `yaml:"arp-validate,omitempty"`
	ARPAllTargets      IntString `yaml:"arp-all-targets,omitempty"`
	UpDelay            IntString `yaml:"up-delay,omitempty"`
	DownDelay          IntString `yaml:"down-delay,omitempty"`
	FailOverMACPolicy  IntString `yaml:"fail-over-mac-policy,omitempty"`
	// Netplan misspelled this as 'gratuitious-arp', not sure if it works with that name.
	// We may need custom handling of both spellings.
	GratuitousARP         *int      `yaml:"gratuitious-arp,omitempty"` // nolint: misspell
	PacketsPerSlave       *int      `yaml:"packets-per-slave,omitempty"`
	PrimaryReselectPolicy IntString `yaml:"primary-reselect-policy,omitempty"`
	ResendIGMP            *int      `yaml:"resend-igmp,omitempty"`
	// bonding.txt says that this can be a value from 1-0x7fffffff, should we be forcing it to be a hex value?
	LearnPacketInterval *int   `yaml:"learn-packet-interval,omitempty"`
	Primary             string `yaml:"primary,omitempty"`
}

// BridgeEthernetById takes a deviceId and creates a bridge with this device
// using this devices config
func (np *Netplan) BridgeEthernetById(deviceId string, bridgeName string) (err error) {
	ethernet, ok := np.Network.Ethernets[deviceId]
	if !ok {
		return errors.NotFoundf("ethernet device with id %q for bridge %q", deviceId, bridgeName)
	}
	shouldCreate, err := np.shouldCreateBridge(deviceId, bridgeName)
	if !shouldCreate {
		// err may be nil, but we shouldn't continue creating
		return errors.Trace(err)
	}
	np.createBridgeFromInterface(bridgeName, deviceId, &ethernet.Interface)
	np.Network.Ethernets[deviceId] = ethernet
	return nil
}

// BridgeVLANById takes a deviceId and creates a bridge with this device
// using this devices config
func (np *Netplan) BridgeVLANById(deviceId string, bridgeName string) (err error) {
	vlan, ok := np.Network.VLANs[deviceId]
	if !ok {
		return errors.NotFoundf("VLAN device with id %q for bridge %q", deviceId, bridgeName)
	}
	shouldCreate, err := np.shouldCreateBridge(deviceId, bridgeName)
	if !shouldCreate {
		// err may be nil, but we shouldn't continue creating
		return errors.Trace(err)
	}
	np.createBridgeFromInterface(bridgeName, deviceId, &vlan.Interface)
	np.Network.VLANs[deviceId] = vlan
	return nil
}

// BridgeBondById takes a deviceId and creates a bridge with this device
// using this devices config
func (np *Netplan) BridgeBondById(deviceId string, bridgeName string) (err error) {
	bond, ok := np.Network.Bonds[deviceId]
	if !ok {
		return errors.NotFoundf("bond device with id %q for bridge %q", deviceId, bridgeName)
	}
	shouldCreate, err := np.shouldCreateBridge(deviceId, bridgeName)
	if !shouldCreate {
		// err may be nil, but we shouldn't continue creating
		return errors.Trace(err)
	}
	np.createBridgeFromInterface(bridgeName, deviceId, &bond.Interface)
	np.Network.Bonds[deviceId] = bond
	return nil
}

// shouldCreateBridge returns true only if it is clear the bridge doesn't already exist, and that the existing device
// isn't in a different bridge.
func (np *Netplan) shouldCreateBridge(deviceId string, bridgeName string) (bool, error) {
	for bName, bridge := range np.Network.Bridges {
		for _, i := range bridge.Interfaces {
			if i == deviceId {
				// The device is already properly bridged, nothing to do
				if bridgeName == bName {
					return false, nil
				} else {
					return false, errors.AlreadyExistsf("cannot create bridge %q, device %q in bridge %q", bridgeName, deviceId, bName)
				}
			}
		}
		if bridgeName == bName {
			return false, errors.AlreadyExistsf(
				"cannot create bridge %q with device %q - bridge %q w/ interfaces %q",
				bridgeName, deviceId, bridgeName, strings.Join(bridge.Interfaces, ", "))
		}
	}
	return true, nil
}

// createBridgeFromInterface will create a bridge stealing the interface details, and wiping the existing interface
// except for MTU so that IP Address information is never duplicated.
func (np *Netplan) createBridgeFromInterface(bridgeName, deviceId string, intf *Interface) {
	if np.Network.Bridges == nil {
		np.Network.Bridges = make(map[string]Bridge)
	}
	np.Network.Bridges[bridgeName] = Bridge{
		Interfaces: []string{deviceId},
		Interface:  *intf,
	}
	*intf = Interface{MTU: intf.MTU}
}

// Marshal a Netplan instance into YAML.
func Marshal(in *Netplan) (out []byte, err error) {
	return goyaml.Marshal(in)
}

type sortableDirEntries []os.DirEntry

func (fil sortableDirEntries) Len() int {
	return len(fil)
}

func (fil sortableDirEntries) Less(i, j int) bool {
	return fil[i].Name() < fil[j].Name()
}

func (fil sortableDirEntries) Swap(i, j int) {
	fil[i], fil[j] = fil[j], fil[i]
}

// ReadDirectory reads the contents of a netplan directory and
// returns complete config.
func ReadDirectory(dirPath string) (np Netplan, err error) {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return np, err
	}
	np.sourceDirectory = dirPath
	sortedDirEntries := sortableDirEntries(dirEntries)
	sort.Sort(sortedDirEntries)

	// First, unmarshal all configuration files into maps and merge them.
	// Since the file list is pre-sorted, the first unmarshalled file
	// serves as the base configuration; subsequent configuration maps are
	// merged into it.
	var mergedConfig map[interface{}]interface{}
	for _, dirEntry := range sortedDirEntries {
		if !dirEntry.IsDir() && strings.HasSuffix(dirEntry.Name(), ".yaml") {
			np.sourceFiles = append(np.sourceFiles, dirEntry.Name())

			pathToConfig := path.Join(np.sourceDirectory, dirEntry.Name())
			configContents, err := os.ReadFile(pathToConfig)
			if err != nil {
				return Netplan{}, errors.Annotatef(err, "reading netplan configuration from %q", pathToConfig)
			}

			var unmarshaledContents map[interface{}]interface{}
			if err := goyaml.Unmarshal(configContents, &unmarshaledContents); err != nil {
				return Netplan{}, errors.Annotatef(err, "unmarshaling netplan configuration from %q", pathToConfig)

			}

			if mergedConfig == nil {
				mergedConfig = unmarshaledContents
			} else {
				mergedResult, err := mergeNetplanConfigs(mergedConfig, unmarshaledContents)
				if err != nil {
					return Netplan{}, errors.Annotatef(err, "merging netplan configuration from %s", pathToConfig)
				}

				// mergeNetplanConfigs should return back the
				// value we passed in. However, lets be extra
				// paranoid and double check that a malicious
				// file did not mutate the type of the returned
				// value.
				mergedConfigMap, ok := mergedResult.(map[interface{}]interface{})
				if !ok {
					return Netplan{}, errors.Errorf("merging netplan configuration from %s caused the original configuration to become corrupted", pathToConfig)
				}
				mergedConfig = mergedConfigMap
			}
		}
	}

	// Serialize the merged config back into yaml and unmarshal it using
	// strict mode it to ensure that the presence of any unknown field
	// triggers an error.
	//
	// As juju mutates the unmashaled Netplan struct and writes it back to
	// disk, using strict mode guarantees that we will never accidentally
	// drop netplan config values just because they were not defined in
	// our structs.
	mergedYAML, err := goyaml.Marshal(mergedConfig)
	if err != nil {
		return Netplan{}, errors.Trace(err)
	} else if err := goyaml.UnmarshalStrict(mergedYAML, &np); err != nil {
		return Netplan{}, errors.Trace(err)
	}
	return np, nil
}

// mergeNetplanConfigs recursively merges two netplan configurations where
// values is src will overwrite values in dst based on the rules described in
// http://manpages.ubuntu.com/manpages/groovy/man8/netplan-generate.8.html:
//
// - If  the  values are YAML boolean or scalar values (numbers and strings)
// the old value is overwritten by the new value.
//
// - If the values are sequences, the sequences are concatenated - the new
// values are appended to the old list.
//
// - If the values are mappings, netplan will examine the elements of the
// mappings in turn using these rules.
//
// The function returns back the merged destination object which *may* be
// different than the one that was passed into the function (e.g. if dst was
// a map or slice that got resized).
func mergeNetplanConfigs(dst, src interface{}) (interface{}, error) {
	if dst == nil {
		return src, nil
	}

	var err error
	switch dstVal := dst.(type) {
	case map[interface{}]interface{}:
		srcVal, ok := src.(map[interface{}]interface{})
		if !ok {
			return nil, errors.Errorf("configuration values have different types (destination: %T, src: %T)", dst, src)
		}

		// Overwrite values in dst for keys that are present in both
		// dst and src.
		for dstMapKey, dstMapVal := range dstVal {
			srcMapVal, exists := srcVal[dstMapKey]
			if !exists {
				continue
			}

			// Merge recursively (if non-scalar values)
			dstVal[dstMapKey], err = mergeNetplanConfigs(dstMapVal, srcMapVal)
			if err != nil {
				return nil, errors.Annotatef(err, "merging configuration key %q", dstMapKey)
			}
		}

		// Now append values from src for keys that are not present in
		// the dst map. However, if the dstVal is nil, just use the
		// srcVal as-is.
		if dstVal == nil {
			return srcVal, nil
		}

		for srcMapKey, srcMapVal := range srcVal {
			_, exists := dstVal[srcMapKey]
			if exists {
				continue
			}

			// Insert new value into the destination.
			dstVal[srcMapKey] = srcMapVal
		}

		return dstVal, nil
	case []interface{}:
		srcVal, ok := src.([]interface{})
		if !ok {
			return nil, errors.Errorf("configuration values have different types (destination: %T, src: %T)", dst, src)
		}

		// Only append missing values to the slice
		dstLen := len(dstVal)
	nextSrcSliceVal:
		for _, srcSliceVal := range srcVal {
			// If the srcSliceVal is not present in any of the
			// original dstVal entries then append it. Note that we
			// don't care about the values that may get potentially
			// appended, hence the pre-calculation of the dstVal
			// length.
			for i := 0; i < dstLen; i++ {
				if reflect.DeepEqual(dstVal[i], srcSliceVal) {
					continue nextSrcSliceVal // value already present
				}
			}

			dstVal = append(dstVal, srcSliceVal)
		}
		return dstVal, nil
	default:
		// Assume a scalar value and overwrite with src
		return src, nil
	}
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
	err = os.WriteFile(tmpFilePath, out, 0644)
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
	return "", errors.NotFoundf("bond device with MAC %q", mac)
}

func (np *Netplan) FindBondByName(name string) (device string, err error) {
	if _, ok := np.Network.Bonds[name]; ok {
		return name, nil
	}
	return "", errors.NotFoundf("bond device with name %q", name)
}

type DeviceType string

const (
	TypeEthernet = DeviceType("ethernet")
	TypeVLAN     = DeviceType("vlan")
	TypeBond     = DeviceType("bond")
)

// FindDeviceByMACOrName will look for an Ethernet, VLAN or Bond matching the Name of the device or its MAC address.
// Name is preferred to MAC address.
func (np *Netplan) FindDeviceByNameOrMAC(name, mac string) (string, DeviceType, error) {
	if name != "" {
		bond, err := np.FindBondByName(name)
		if err == nil {
			return bond, TypeBond, nil
		}
		if !errors.Is(err, errors.NotFound) {
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
	// by MAC is less reliable because things like vlans often have the same MAC address
	if mac != "" {
		bond, err := np.FindBondByMAC(mac)
		if err == nil {
			return bond, TypeBond, nil
		}
		if !errors.Is(err, errors.NotFound) {
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
	return "", "", errors.NotFoundf("device - name %q MAC %q", name, mac)
}
