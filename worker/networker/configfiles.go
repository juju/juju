// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/juju/utils"

	"github.com/juju/juju/network"
)

// ConfigFile defines operations on a network config file for a single
// network interface.
type ConfigFile interface {
	// InterfaceName returns the inteface name for this config file.
	InterfaceName() string

	// FileName returns the full path for storing this config file on
	// disk.
	FileName() string

	// InterfaceInfo returns the network.InterfaceInfo associated with
	// this config file.
	InterfaceInfo() network.InterfaceInfo

	// ReadData opens the underlying config file and populates the
	// data.
	ReadData() error

	// Data returns the original raw contents of this config file.
	Data() []byte

	// RenderManaged generates network config based on the known
	// network.InterfaceInfo and returns it.
	RenderManaged() []byte

	// NeedsUpdating returns true if this config file needs to be
	// written to disk.
	NeedsUpdating() bool

	// IsPendingRemoval returns true if this config file needs to be
	// removed.
	IsPendingRemoval() bool

	// IsManaged returns true if this config file is managed by Juju.
	IsManaged() bool

	// UpdateData updates the internally stored raw contents of this
	// config file, and sets the "needs updating" internal flag,
	// returning true, if newData is different. If newData is the same
	// as the old or the interface is not managed, returns false and
	// does not change anything.
	UpdateData(newData []byte) bool

	// MarkForRemoval marks this config file as pending for removal,
	// if the interface is managed.
	MarkForRemoval()

	// Apply updates the config file data (if it needs updating),
	// removes the file (if it's marked removal), or does nothing.
	Apply() error
}

// ManagedHeader is the header of a network config file managed by Juju.
const ManagedHeader = "# Managed by Juju, please don't change.\n\n"

// RenderMainConfig generates a managed main config file, which
// includes *.cfg individual config files inside configSubDir (i.e.
// /etc/network/interfaces).
func RenderMainConfig(configSubDir string) []byte {
	var data bytes.Buffer
	globSpec := fmt.Sprintf("%s/*.cfg", configSubDir)
	logger.Debugf("rendering main network config to include %q", globSpec)
	fmt.Fprintf(&data, ManagedHeader)
	fmt.Fprintf(&data, "source %s\n\n", globSpec)
	return data.Bytes()
}

// configFile implement ConfigFile.
type configFile struct {
	// interfaceName holds the name of the network interface.
	interfaceName string

	// fileName holds the full path to the config file on disk.
	fileName string

	// interfaceInfo holds the network information about this
	// interface, known by the API server.
	interfaceInfo network.InterfaceInfo

	// data holds the raw file contents of the underlying file.
	data []byte

	// needsUpdating is true when the interface config has changed and
	// needs to be written back to disk.
	needsUpdating bool

	// pendingRemoval is true when the interface config file is about
	// to be removed.
	pendingRemoval bool
}

var _ ConfigFile = (*configFile)(nil)

// InterfaceName implements ConfigFile.InterfaceName().
func (f *configFile) InterfaceName() string {
	return f.interfaceName
}

// FileName implements ConfigFile.FileName().
func (f *configFile) FileName() string {
	return f.fileName
}

// ReadData implements ConfigFile.ReadData().
func (f *configFile) ReadData() error {
	data, err := ioutil.ReadFile(f.fileName)
	if err != nil {
		return err
	}
	f.UpdateData(data)
	return nil
}

// InterfaceInfo implements ConfigFile.InterfaceInfo().
func (f *configFile) InterfaceInfo() network.InterfaceInfo {
	return f.interfaceInfo
}

// Data implements ConfigFile.Data().
func (f *configFile) Data() []byte {
	return f.data
}

// RenderManaged implements ConfigFile.RenderManaged().
//
// TODO(dimitern) Once container addressability work has progressed
// enough, modify this to render the config taking all fields of
// network.InterfaceInfo into account.
func (f *configFile) RenderManaged() []byte {
	var data bytes.Buffer
	actualName := f.interfaceInfo.ActualInterfaceName()
	logger.Debugf("rendering managed config for %q", actualName)
	fmt.Fprintf(&data, ManagedHeader)
	fmt.Fprintf(&data, "auto %s\n", actualName)
	fmt.Fprintf(&data, "iface %s inet dhcp\n", actualName)

	// Add vlan-raw-device line for VLAN interfaces.
	if f.interfaceInfo.IsVLAN() {
		// network.InterfaceInfo.InterfaceName is always the physical
		// device name, i.e. "eth1" for VLAN interface "eth1.42".
		fmt.Fprintf(&data, "\tvlan-raw-device %s\n", f.interfaceInfo.InterfaceName)
	}
	fmt.Fprintf(&data, "\n")
	return data.Bytes()
}

// NeedsUpdating implements ConfigFile.NeedsUpdating().
func (f *configFile) NeedsUpdating() bool {
	return f.needsUpdating
}

// IsPendingRemoval implements ConfigFile.IsPendingRemoval().
func (f *configFile) IsPendingRemoval() bool {
	return f.pendingRemoval
}

// IsManaged implements ConfigFile.IsManaged()
func (f *configFile) IsManaged() bool {
	return len(f.data) > 0 && bytes.HasPrefix(f.data, []byte(ManagedHeader))
}

// UpdateData implements ConfigFile.UpdateData().
func (f *configFile) UpdateData(newData []byte) bool {
	if bytes.Equal(f.data, newData) {
		// Not changed.
		if f.interfaceName == "" {
			// This is the main config.
			logger.Debugf("main network config not changed")
		} else {
			logger.Debugf("network config for %q not changed", f.interfaceName)
		}
		return false
	}
	f.data = make([]byte, len(newData))
	copy(f.data, newData)
	f.needsUpdating = true
	return true
}

// MarkForRemoval implements ConfigFile.MarkForRemoval().
func (f *configFile) MarkForRemoval() {
	f.pendingRemoval = true
}

// Apply implements ConfigFile.Apply().
func (f *configFile) Apply() error {
	if f.needsUpdating {
		err := utils.AtomicWriteFile(f.fileName, f.data, 0644)
		if err != nil {
			logger.Errorf("failed to write file %q: %v", f.fileName, err)
			return err
		}
		if f.interfaceName == "" {
			logger.Debugf("updated main network config %q", f.fileName)
		} else {
			logger.Debugf("updated network config %q for %q", f.fileName, f.interfaceName)
		}
		f.needsUpdating = false
	}
	if f.pendingRemoval {
		err := os.Remove(f.fileName)
		if err != nil {
			logger.Errorf("failed to remove file %q: %v", f.fileName, err)
			return err
		}
		logger.Debugf("removed config %q for %q", f.fileName, f.interfaceName)
		f.pendingRemoval = false
	}
	return nil
}
