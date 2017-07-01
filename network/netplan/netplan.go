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
type Interface struct {
	Dhcp4       bool        `yaml:"dhcp4,omitempty"`
	Dhcp6       bool        `yaml:"dhcp6,omitempty"`
	Addresses   []string    `yaml:"addresses,omitempty"`
	Gateway4    string      `yaml:"gateway4,omitempty"`
	Gateway6    string      `yaml:"gateway6,omitempty"`
	Nameservers Nameservers `yaml:"nameservers,omitempty"`
	MTU         int         `yaml:"mtu,omitempty"`
}
type Ethernet struct {
	Match     map[string]string `yaml:"match"`
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
	AccessPoints map[string]AccessPoint `yaml:"access-points,omitempty"`
	Interface    `yaml:",inline"`
}

type Bridge struct {
	Interfaces []string `yaml:"interfaces,omitempty,flow"`
	Interface  `yaml:",inline"`
}

type Route struct {
	To     string `yaml:"to,omitempty"`
	Via    string `yaml:"via,omitempty"`
	Metric int    `yaml:"metric,omitempty"`
}

type Network struct {
	Version   int                 `yaml:"version"`
	Renderer  string              `yaml:"renderer,omitempty"`
	Ethernets map[string]Ethernet `yaml:"ethernets,omitempty"`
	Wifis     map[string]Wifi     `yaml:"wifis,omitempty"`
	Bridges   map[string]Bridge   `yaml:"bridges,omitempty"`
	Routes    []Route             `yaml:"routes,omitempty"`
}

type Netplan struct {
	Network         Network `yaml:"network"`
	sourceDirectory string
	sourceFiles     []string
	backedFiles     map[string]string
	writtenFile     string
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
	}
	return "", errors.NotFoundf("Ethernet device with mac %q", mac)
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
	return "", errors.NotFoundf("Ethernet device with name %q", name)
}
