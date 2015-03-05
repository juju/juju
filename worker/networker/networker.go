// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	apinetworker "github.com/juju/juju/api/networker"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.networker")

// DefaultConfigBaseDir is the usual root directory where the
// network configuration is kept.
const DefaultConfigBaseDir = "/etc/network"

// Networker configures network interfaces on the machine, as needed.
type Networker struct {
	tomb tomb.Tomb

	st  apinetworker.State
	tag names.MachineTag

	// isVLANSupportInstalled is set to true when the VLAN kernel
	// module 8021q was installed.
	isVLANSupportInstalled bool

	// intrusiveMode determines whether to write any changes
	// to the network config (intrusive mode) or not (non-intrusive mode).
	intrusiveMode bool

	// configBasePath is the root directory where the networking
	// config is kept (usually /etc/network).
	configBaseDir string

	// primaryInterface is the name of the primary network interface
	// on the machine (usually "eth0").
	primaryInterface string

	// loopbackInterface is the name of the loopback interface on the
	// machine (usually "lo").
	loopbackInterface string

	// configFiles holds all loaded network config files, using the
	// full file path as key.
	configFiles map[string]*configFile

	// interfaceInfo holds the info for all network interfaces
	// discovered via the API, using the interface name as key.
	interfaceInfo map[string]network.InterfaceInfo

	// interfaces holds all known network interfaces on the machine,
	// using their name as key.
	interfaces map[string]net.Interface

	// commands holds generated scripts (e.g. for bringing interfaces
	// up or down, etc.) which were not executed yet.
	commands []string
}

var _ worker.Worker = (*Networker)(nil)

// NewNetworker returns a Worker that handles machine networking
// configuration. If there is no <configBasePath>/interfaces file, an
// error is returned.
func NewNetworker(
	st apinetworker.State,
	agentConfig agent.Config,
	intrusiveMode bool,
	configBaseDir string,
) (*Networker, error) {
	tag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		// This should never happen, as there is a check for it in the
		// machine agent.
		return nil, fmt.Errorf("expected names.MachineTag, got %T", agentConfig.Tag())
	}
	nw := &Networker{
		st:            st,
		tag:           tag,
		intrusiveMode: intrusiveMode,
		configBaseDir: configBaseDir,
		configFiles:   make(map[string]*configFile),
		interfaceInfo: make(map[string]network.InterfaceInfo),
		interfaces:    make(map[string]net.Interface),
	}
	go func() {
		defer nw.tomb.Done()
		nw.tomb.Kill(nw.loop())
	}()
	return nw, nil
}

// Kill implements Worker.Kill().
func (nw *Networker) Kill() {
	nw.tomb.Kill(nil)
}

// Wait implements Worker.Wait().
func (nw *Networker) Wait() error {
	return nw.tomb.Wait()
}

// ConfigBaseDir returns the root directory where the networking config is
// kept. Usually, this is /etc/network.
func (nw *Networker) ConfigBaseDir() string {
	return nw.configBaseDir
}

// ConfigSubDir returns the directory where individual config files
// for each network interface are kept. Usually, this is
// /etc/network/interfaces.d.
func (nw *Networker) ConfigSubDir() string {
	return filepath.Join(nw.ConfigBaseDir(), "interfaces.d")
}

// ConfigFile returns the full path to the network config file for the
// given interface. If interfaceName is "", the path to the main
// network config file is returned (usually, this is
// /etc/network/interfaces).
func (nw *Networker) ConfigFile(interfaceName string) string {
	if interfaceName == "" {
		return filepath.Join(nw.ConfigBaseDir(), "interfaces")
	}
	return filepath.Join(nw.ConfigSubDir(), interfaceName+".cfg")
}

// IntrusiveMode returns whether the networker is changing networking
// configuration files (intrusive mode) or won't modify them on the
// machine (non-intrusive mode).
func (nw *Networker) IntrusiveMode() bool {
	return nw.intrusiveMode
}

// IsPrimaryInterfaceOrLoopback returns whether the given
// interfaceName matches the primary or loopback network interface.
func (nw *Networker) IsPrimaryInterfaceOrLoopback(interfaceName string) bool {
	return interfaceName == nw.primaryInterface ||
		interfaceName == nw.loopbackInterface
}

// loop is the worker's main loop.
func (nw *Networker) loop() error {
	// TODO(dimitern) Networker is disabled until we have time to fix
	// it so it's not overwriting /etc/network/interfaces
	// indiscriminately for containers and possibly other cases.
	logger.Infof("networker is disabled - not starting on machine %q", nw.tag)
	return nil

	logger.Debugf("starting on machine %q", nw.tag)
	if !nw.IntrusiveMode() {
		logger.Warningf("running in non-intrusive mode - no commands or changes to network config will be done")
	}
	w, err := nw.init()
	if err != nil {
		if w != nil {
			// We don't bother to propagate an error, because we
			// already have an error
			w.Stop()
		}
		return err
	}
	defer watcher.Stop(w, &nw.tomb)
	logger.Debugf("initialized and started watching")
	for {
		select {
		case <-nw.tomb.Dying():
			logger.Debugf("shutting down")
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			logger.Debugf("got change notification")
			if !ok {
				return watcher.EnsureErr(w)
			}
			if err := nw.handle(); err != nil {
				return err
			}
		}
	}
}

// init initializes the worker and starts a watcher for monitoring
// network interface changes.
func (nw *Networker) init() (apiwatcher.NotifyWatcher, error) {
	// Discover all interfaces on the machine and populate internal
	// maps, reading existing config files as well, and fetch the
	// network info from the API..
	if err := nw.updateInterfaces(); err != nil {
		return nil, err
	}

	// Apply changes (i.e. write managed config files and load the
	// VLAN module if needed).
	if err := nw.applyAndExecute(); err != nil {
		return nil, err
	}
	return nw.st.WatchInterfaces(nw.tag)
}

// handle processes changes to network interfaces in state.
func (nw *Networker) handle() error {
	// Update interfaces and config files as needed.
	if err := nw.updateInterfaces(); err != nil {
		return err
	}

	// Bring down disabled interfaces.
	nw.prepareDownCommands()

	// Bring up configured interfaces.
	nw.prepareUpCommands()

	// Apply any needed changes to config and run generated commands.
	if err := nw.applyAndExecute(); err != nil {
		return err
	}
	return nil
}

// updateInterfaces discovers all known network interfaces on the
// machine and caches the result internally.
func (nw *Networker) updateInterfaces() error {
	interfaces, err := Interfaces()
	if err != nil {
		return fmt.Errorf("cannot retrieve network interfaces: %v", err)
	}
	logger.Debugf("updated machine network interfaces info")

	// Read the main config file first.
	mainConfig := nw.ConfigFile("")
	if _, ok := nw.configFiles[mainConfig]; !ok {
		if err := nw.readConfig("", mainConfig); err != nil {
			return err
		}
	}

	// Populate the internal maps for interfaces and configFiles and
	// find the primary interface.
	nw.interfaces = make(map[string]net.Interface)
	for _, iface := range interfaces {
		logger.Debugf(
			"found interface %q with index %d and flags %s",
			iface.Name,
			iface.Index,
			iface.Flags.String(),
		)
		nw.interfaces[iface.Name] = iface
		fullPath := nw.ConfigFile(iface.Name)
		if _, ok := nw.configFiles[fullPath]; !ok {
			if err := nw.readConfig(iface.Name, fullPath); err != nil {
				return err
			}
		}
		if iface.Flags&net.FlagLoopback != 0 && nw.loopbackInterface == "" {
			nw.loopbackInterface = iface.Name
			logger.Debugf("loopback interface is %q", iface.Name)
			continue
		}

		// The first enabled, non-loopback interface should be the
		// primary.
		if iface.Flags&net.FlagUp != 0 && nw.primaryInterface == "" {
			nw.primaryInterface = iface.Name
			logger.Debugf("primary interface is %q", iface.Name)
		}
	}

	// Fetch network info from the API and generate managed config as
	// needed.
	if err := nw.fetchInterfaceInfo(); err != nil {
		return err
	}

	return nil
}

// fetchInterfaceInfo makes an API call to get all known
// *network.InterfaceInfo entries for each interface on the machine.
// If there are any VLAN interfaces to setup, it also generates
// commands to load the kernal 8021q VLAN module, if not already
// loaded and when not running inside an LXC container.
func (nw *Networker) fetchInterfaceInfo() error {
	interfaceInfo, err := nw.st.MachineNetworkConfig(nw.tag)
	if err != nil {
		logger.Errorf("failed to retrieve network info: %v", err)
		return err
	}
	logger.Debugf("fetched known network info from state")

	haveVLANs := false
	nw.interfaceInfo = make(map[string]network.InterfaceInfo)
	for _, info := range interfaceInfo {
		actualName := info.ActualInterfaceName()
		logger.Debugf(
			"have network info for %q: MAC=%q, disabled: %v, vlan-tag: %d",
			actualName,
			info.MACAddress,
			info.Disabled,
			info.VLANTag,
		)
		if info.IsVLAN() {
			haveVLANs = true
		}
		nw.interfaceInfo[actualName] = info
		fullPath := nw.ConfigFile(actualName)
		cfgFile, ok := nw.configFiles[fullPath]
		if !ok {
			// We have info for an interface which is was not
			// discovered on the machine, so we need to add it
			// list of managed interfaces.
			logger.Debugf("no config for %q but network info exists; will generate", actualName)
			if err := nw.readConfig(actualName, fullPath); err != nil {
				return err
			}
			cfgFile = nw.configFiles[fullPath]
		}
		cfgFile.interfaceInfo = info

		// Make sure we generate managed config, in case it changed.
		cfgFile.UpdateData(cfgFile.RenderManaged())

		nw.configFiles[fullPath] = cfgFile
	}

	// Generate managed main config file.
	cfgFile := nw.configFiles[nw.ConfigFile("")]
	cfgFile.UpdateData(RenderMainConfig(nw.ConfigSubDir()))

	if !haveVLANs {
		return nil
	}

	if !nw.isVLANSupportInstalled {
		if nw.isRunningInLXC() {
			msg := "running inside LXC: "
			msg += "cannot load the required 8021q kernel module for VLAN support; "
			msg += "please ensure it is loaded on the host"
			logger.Warningf(msg)
			return nil
		}
		nw.prepareVLANModule()
		nw.isVLANSupportInstalled = true
		logger.Debugf("need to load VLAN 8021q kernel module")
	}
	return nil
}

// applyAndExecute updates or removes config files as needed, and runs
// all accumulated pending commands, and if all commands succeed,
// resets the commands slice. If the networker is running in "safe
// mode" nothing is changed.
func (nw *Networker) applyAndExecute() error {
	if !nw.IntrusiveMode() {
		logger.Warningf("running in non-intrusive mode - no changes made")
		return nil
	}

	// Create the config subdir, if needed.
	configSubDir := nw.ConfigSubDir()
	if _, err := os.Stat(configSubDir); err != nil {
		if err := os.Mkdir(configSubDir, 0755); err != nil {
			logger.Errorf("failed to create directory %q: %v", configSubDir, err)
			return err
		}
	}

	// Read config subdir contents and remove any non-managed files.
	files, err := ioutil.ReadDir(configSubDir)
	if err != nil {
		logger.Errorf("failed to read directory %q: %v", configSubDir, err)
		return err
	}
	for _, info := range files {
		if !info.Mode().IsRegular() {
			// Skip special files and directories.
			continue
		}
		fullPath := filepath.Join(configSubDir, info.Name())
		if _, ok := nw.configFiles[fullPath]; !ok {
			if err := os.Remove(fullPath); err != nil {
				logger.Errorf("failed to remove non-managed config %q: %v", fullPath, err)
				return err
			}
		}
	}

	// Apply all changes needed for each config file.
	logger.Debugf("applying changes to config files as needed")
	for _, cfgFile := range nw.configFiles {
		if err := cfgFile.Apply(); err != nil {
			return err
		}
	}
	if len(nw.commands) > 0 {
		logger.Debugf("executing commands %v", nw.commands)
		if err := ExecuteCommands(nw.commands); err != nil {
			return err
		}
		nw.commands = []string{}
	}
	return nil
}

// isRunningInLXC returns whether the worker is running inside a LXC
// container or not. When running in LXC containers, we should not
// attempt to modprobe anything, as it's not possible and leads to
// run-time errors. See http://pad.lv/1353443.
func (nw *Networker) isRunningInLXC() bool {
	// In case of nested containers, we need to check
	// the last nesting level to ensure it's not LXC.
	machineId := strings.ToLower(nw.tag.Id())
	parts := strings.Split(machineId, "/")
	return len(parts) > 2 && parts[len(parts)-2] == "lxc"
}

// prepareVLANModule generates the necessary commands to load the VLAN
// kernel module 8021q.
func (nw *Networker) prepareVLANModule() {
	commands := []string{
		`dpkg-query -s vlan || apt-get --option Dpkg::Options::=--force-confold --assume-yes install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	nw.commands = append(nw.commands, commands...)
}

// prepareUpCommands generates ifup commands to bring the needed
// interfaces up.
func (nw *Networker) prepareUpCommands() {
	bringUp := []string{}
	logger.Debugf("preparing to bring interfaces up")
	for name, info := range nw.interfaceInfo {
		if nw.IsPrimaryInterfaceOrLoopback(name) {
			logger.Debugf("skipping primary or loopback interface %q", name)
			continue
		}
		fullPath := nw.ConfigFile(name)
		cfgFile := nw.configFiles[fullPath]
		if info.Disabled && !cfgFile.IsPendingRemoval() {
			cfgFile.MarkForRemoval()
			logger.Debugf("disabled %q marked for removal", name)
		} else if !info.Disabled && !InterfaceIsUp(name) {
			bringUp = append(bringUp, name)
			logger.Debugf("will bring %q up", name)
		}
	}

	// Sort interfaces to ensure raw interfaces go before their
	// virtual dependents (i.e. VLANs)
	sort.Sort(sort.StringSlice(bringUp))
	for _, name := range bringUp {
		nw.commands = append(nw.commands, "ifup "+name)
	}
}

// prepareUpCommands generates ifdown commands to bring the needed
// interfaces down.
func (nw *Networker) prepareDownCommands() {
	bringDown := []string{}
	logger.Debugf("preparing to bring interfaces down")
	for _, cfgFile := range nw.configFiles {
		name := cfgFile.InterfaceName()
		if name == "" {
			// Skip the main config file.
			continue
		}
		if nw.IsPrimaryInterfaceOrLoopback(name) {
			logger.Debugf("skipping primary or loopback interface %q", name)
			continue
		}
		info := cfgFile.InterfaceInfo()
		if info.Disabled {
			if InterfaceIsUp(name) {
				bringDown = append(bringDown, name)
				logger.Debugf("will bring %q down", name)
			}
			if !cfgFile.IsPendingRemoval() {
				cfgFile.MarkForRemoval()
				logger.Debugf("diabled %q marked for removal", name)
			}
		}
	}

	// Sort interfaces to ensure raw interfaces go after their virtual
	// dependents (i.e. VLANs)
	sort.Sort(sort.Reverse(sort.StringSlice(bringDown)))
	for _, name := range bringDown {
		nw.commands = append(nw.commands, "ifdown "+name)
	}
}

// readConfig populates the configFiles map with an entry for the
// given interface and filename, and tries to read the file. If the
// config file is missing, that's OK, as it will be generated later
// and it's not considered an error. If configFiles already contains
// an entry for fileName, nothing is changed.
func (nw *Networker) readConfig(interfaceName, fileName string) error {
	cfgFile := &configFile{
		interfaceName: interfaceName,
		fileName:      fileName,
	}
	if err := cfgFile.ReadData(); !os.IsNotExist(err) && err != nil {
		return err
	}
	if _, ok := nw.configFiles[fileName]; !ok {
		nw.configFiles[fileName] = cfgFile
	}
	return nil
}
