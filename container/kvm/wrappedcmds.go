// Copyright 2013-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

// This file contains wrappers around the following executables:
//   genisoimage
//   qemu-img
//   virsh
// Those executables are found in the following packages:
//   genisoimage
//   libvirt-bin
//   qemu-utils
//
// These executables provide Juju's interface to dealing with kvm containers.
// They are the means by which we start, stop and list running containers on
// the host

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v1"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"

	"github.com/juju/juju/container/kvm/libvirt"
	"github.com/juju/juju/juju/paths"
)

const (
	guestDir = "guests"
	poolName = "juju-pool"
	kvm      = "kvm"
	metadata = "meta-data"
	userdata = "user-data"

	// This path is only valid on ubuntu, and xenial at this point.
	// TODO(ro) 2017-01-20 Determine if we will support trusty and update this
	// as necessary if so. It seems it will require some serious acrobatics to
	// get trusty to work properly and that may be out of scope for juju.
	nvramCode = "/usr/share/AAVMF/AAVMF_CODE.fd"
)

var (
	// The regular expression for breaking up the results of 'virsh list'
	// (?m) - specify that this is a multi-line regex
	// first part is the opaque identifier we don't care about
	// then the hostname, and lastly the status.
	machineListPattern = regexp.MustCompile(`(?m)^\s+\d+\s+(?P<hostname>[-\w]+)\s+(?P<status>.+)\s*$`)
)

// CreateMachineParams Implements libvirt.domainParams.
type CreateMachineParams struct {
	Hostname      string
	Series        string
	UserDataFile  string
	NetworkBridge string
	Memory        uint64
	CpuCores      uint64
	RootDisk      uint64
	Interfaces    []libvirt.InterfaceInfo

	nvram string

	disks    []libvirt.DiskInfo
	findPath func(string) (string, error)

	runCmd       runFunc
	runCmdAsRoot runFunc
	arch         string
}

// Arch returns the architecture to be used.
func (p CreateMachineParams) Arch() string {
	if p.arch != "" {
		return p.arch
	}
	return arch.HostArch()
}

// Loader is the path to the binary firmware blob used in UEFI booting. At the
// time of this writing only ARM64 requires this to run.
func (p CreateMachineParams) Loader() string {
	return nvramCode
}

// NVRAM is the path to the "pflash" drive where UEFI stores variables related
// to booting an image. At the time of this writing only ARM64 uses this.
func (p CreateMachineParams) NVRAM() string {
	return p.nvram
}

// Host implements libvirt.domainParams.
func (p CreateMachineParams) Host() string {
	return p.Hostname
}

// CPUs implements libvirt.domainParams.
func (p CreateMachineParams) CPUs() uint64 {
	if p.CpuCores == 0 {
		return 1
	}
	return p.CpuCores
}

// DiskInfo implements libvirt.domainParams.
func (p CreateMachineParams) DiskInfo() []libvirt.DiskInfo {
	return p.disks
}

// RAM implements libvirt.domainParams.
func (p CreateMachineParams) RAM() uint64 {
	if p.Memory == 0 {
		return 512
	}
	return p.Memory
}

// NetworkInfo implements libvirt.domainParams.
func (p CreateMachineParams) NetworkInfo() []libvirt.InterfaceInfo {
	return p.Interfaces
}

// ValidateDomainParams implements libvirt.domainParams.
func (p CreateMachineParams) ValidateDomainParams() error {
	if p.Hostname == "" {
		return errors.Errorf("missing required hostname")
	}
	if len(p.disks) < 2 {
		// We need at least the drive and the data source disk.
		return errors.Errorf("got %d disks, need at least 2", len(p.disks))
	}
	var ds, fs bool
	for _, d := range p.disks {
		if d.Driver() == "qcow2" {
			fs = true
		}
		if d.Driver() == "raw" {
			ds = true
		}
	}
	if !ds {
		return errors.Trace(errors.Errorf("missing data source disk"))
	}
	if !fs {
		return errors.Trace(errors.Errorf("missing system disk"))
	}
	return nil
}

// diskInfo is type for imlementing libvirt.DiskInfo.
type diskInfo struct {
	driver, source string
}

// Driver implements libvirt.DiskInfo.
func (d diskInfo) Driver() string {
	return d.driver
}

// Source implements libvirt.Source.
func (d diskInfo) Source() string {
	return d.source
}

// CreateMachine creates a virtual machine and starts it.
func CreateMachine(params CreateMachineParams) error {
	if params.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	setDefaults(&params)

	templateDir := filepath.Dir(params.UserDataFile)

	err := writeMetadata(templateDir)
	if err != nil {
		return errors.Annotate(err, "failed to write instance metadata")
	}

	if err = createNVRAM(params); err != nil {
		return errors.Annotatef(err, "failed to create NVRAM on %q for %q", params.Arch(), params.Host())
	}

	dsPath, err := writeDatasourceVolume(params)
	if err != nil {
		return errors.Annotatef(err, "failed to write data source volume for %q", params.Host())
	}

	imgPath, err := writeRootDisk(params)
	if err != nil {
		return errors.Annotatef(err, "failed to write root volume for %q", params.Host())
	}

	params.disks = append(params.disks, diskInfo{source: imgPath, driver: "qcow2"})
	params.disks = append(params.disks, diskInfo{source: dsPath, driver: "raw"})

	domainPath, err := writeDomainXML(templateDir, params)
	if err != nil {
		return errors.Annotatef(err, "failed to write domain xml for %q", params.Host())
	}

	out, err := params.runCmdAsRoot("virsh", "define", domainPath)
	if err != nil {
		return errors.Annotatef(err, "failed to defined the domain for %q from %s", params.Host(), domainPath)
	}
	logger.Debugf("created domain: %s", out)

	out, err = params.runCmdAsRoot("virsh", "start", params.Host())
	if err != nil {
		return errors.Annotatef(err, "failed to start domain %q", params.Host())
	}
	logger.Debugf("started domain: %s", out)

	return err
}

// Setup the default values for params.
func setDefaults(p *CreateMachineParams) {
	if p.findPath == nil {
		p.findPath = paths.DataDir
	}
	if p.runCmd == nil {
		p.runCmd = runAsLibvirt
	}
	if p.runCmdAsRoot == nil {
		p.runCmdAsRoot = run
	}
}

// DestroyMachine destroys the virtual machine represented by the kvmContainer.
func DestroyMachine(c *kvmContainer) error {
	if c.runCmd == nil {
		c.runCmd = run
	}
	if c.pathfinder == nil {
		c.pathfinder = paths.DataDir
	}

	// We don't return errors for virsh commands because it is possible that we
	// didn't succeed in creating the domain. Additionally, we want all the
	// commands to run. If any fail it is certainly because the thing we're
	// trying to remove wasn't created. However, we still want to try removing
	// all the parts. The exception here is getting the guestBase, if that
	// fails we return the error because we cannot continue without it.

	_, err := c.runCmd("virsh", "destroy", c.Name())
	if err != nil {
		logger.Infof("`virsh destroy %s` failed: %q", c.Name(), err)
	}

	_, err = c.runCmd("virsh", "undefine", "--nvram", c.Name())
	if err != nil {
		logger.Infof("`virsh undefine %s` failed: %q", c.Name(), err)
	}
	guestBase, err := guestPath(c.pathfinder)
	if err != nil {
		return errors.Trace(err)
	}
	err = os.Remove(filepath.Join(guestBase, fmt.Sprintf("%s.qcow", c.Name())))
	if err != nil {
		logger.Errorf("failed to remove system disk for %q: %s", c.Name(), err)
	}
	err = os.Remove(filepath.Join(guestBase, fmt.Sprintf("%s-ds.iso", c.Name())))
	if err != nil {
		logger.Errorf("failed to remove cloud-init data disk for %q: %s", c.Name(), err)
	}

	return nil
}

// AutostartMachine indicates that the virtual machines should automatically
// restart when the host restarts.
func AutostartMachine(c *kvmContainer) error {
	if c.runCmd == nil {
		c.runCmd = run
	}
	_, err := c.runCmd("virsh", "autostart", c.Name())
	return errors.Annotatef(err, "failed to autostart domain %q", c.Name())
}

// ListMachines returns a map of machine name to state, where state is one of:
// running, idle, paused, shutdown, shut off, crashed, dying, pmsuspended.
func ListMachines(runCmd runFunc) (map[string]string, error) {
	if runCmd == nil {
		runCmd = run
	}

	output, err := runCmd("virsh", "-q", "list", "--all")
	if err != nil {
		return nil, err
	}
	// Split the output into lines.
	// Regex matching is the easiest way to match the lines.
	//   id hostname status
	// separated by whitespace, with whitespace at the start too.
	result := make(map[string]string)
	for _, s := range machineListPattern.FindAllStringSubmatchIndex(output, -1) {
		hostnameAndStatus := machineListPattern.ExpandString(nil, "$hostname $status", output, s)
		parts := strings.SplitN(string(hostnameAndStatus), " ", 2)
		result[parts[0]] = parts[1]
	}
	return result, nil
}

// guestPath returns the path to the guest directory from the given
// pathfinder.
func guestPath(pathfinder func(string) (string, error)) (string, error) {
	baseDir, err := pathfinder(series.MustHostSeries())
	if err != nil {
		return "", errors.Trace(err)
	}
	return filepath.Join(baseDir, kvm, guestDir), nil
}

// writeDataSourceVolume creates a data source image for cloud init.
func writeDatasourceVolume(params CreateMachineParams) (string, error) {
	templateDir := filepath.Dir(params.UserDataFile)

	if err := writeMetadata(templateDir); err != nil {
		return "", errors.Trace(err)
	}

	// Creating a working DS volume was a bit troublesome for me. I finally
	// found the details in the docs.
	// http://cloudinit.readthedocs.io/en/latest/topics/datasources/nocloud.html
	//
	// The arguments passed to create the DS volume for NoCloud must be
	// `user-data` and `meta-data`. So the `cloud-init` file we generate won't
	// work. Also, they must be exactly `user-data` and `meta-data` with no
	// path beforehand, so `$JUJUDIR/containers/juju-someid-0/user-data` also
	// fails.
	//
	// Furthermore, symlinks aren't followed by NoCloud. So we rename our
	// cloud-init file to user-data. We could change the output name in
	// juju/cloudconfig/containerinit/container_userdata.go:WriteUserData but
	// who knows what that will break.
	userDataPath := filepath.Join(templateDir, userdata)
	if err := os.Rename(params.UserDataFile, userDataPath); err != nil {
		return "", errors.Trace(err)
	}

	// We then change directories and capture our original directory to return
	// to.  This allows us to run the command with user-data and meta-data as
	// relative paths to appease the NoCloud script.
	owd, err := os.Getwd()
	if err != nil {
		return "", errors.Trace(err)
	}
	if err = os.Chdir(templateDir); err != nil {
		return "", errors.Trace(err)
	}

	// Create data the source volume outputting the iso image to the guests
	// (AKA libvirt storage pool) directory.
	guestBase, err := guestPath(params.findPath)
	if err != nil {
		return "", errors.Trace(err)
	}
	dsPath := filepath.Join(guestBase, fmt.Sprintf("%s-ds.iso", params.Host()))

	out, err := params.runCmd(
		"genisoimage",
		"-output", dsPath,
		"-volid", "cidata",
		"-joliet", "-rock",
		userdata,
		metadata)
	if err != nil {
		return "", errors.Trace(err)
	}
	logger.Debugf("create ds image: %s", out)

	// Here we return to the old working directory.
	if err := os.Chdir(owd); err != nil {
		return "", errors.Trace(err)
	}

	return dsPath, nil
}

// writeDomainXML writes out the configuration required to create a new guest
// domain.
func writeDomainXML(templateDir string, p CreateMachineParams) (string, error) {
	domainPath := filepath.Join(templateDir, fmt.Sprintf("%s.xml", p.Host()))
	dom, err := libvirt.NewDomain(p)
	if err != nil {
		return "", errors.Trace(err)
	}

	ml, err := xml.MarshalIndent(&dom, "", "    ")
	if err != nil {
		return "", errors.Trace(err)
	}

	f, err := os.Create(domainPath)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() {
		err = f.Close()
		if err != nil {
			logger.Debugf("failed defer %q", errors.Trace(err))
		}
	}()

	_, err = f.Write(ml)
	if err != nil {
		return "", errors.Trace(err)
	}

	return domainPath, nil
}

// writeMetadata writes out a metadata file with an UUID instance-id. The
// meta-data file is used in the data source image along with user-data nee
// cloud-init. `instance-id` is a required field in meta-data. It is what is
// used to determine if this is the first boot, thereby whether or not to run
// cloud-init.
// See: http://cloudinit.readthedocs.io/en/latest/topics/datasources/nocloud.html
func writeMetadata(dir string) error {
	data := fmt.Sprintf(`{"instance-id": "%s"}`, utils.MustNewUUID())
	f, err := os.Create(filepath.Join(dir, metadata))
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if err = f.Close(); err != nil {
			logger.Errorf("failed to close %q %s", f.Name(), err)
		}
	}()
	_, err = f.WriteString(data)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// writeRootDisk writes out the root disk for the container.  This creates a
// system disk backed by our shared series/arch backing store.
func writeRootDisk(params CreateMachineParams) (string, error) {
	guestBase, err := guestPath(params.findPath)
	if err != nil {
		return "", errors.Trace(err)
	}
	imgPath := filepath.Join(guestBase, fmt.Sprintf("%s.qcow", params.Host()))
	backingPath := filepath.Join(
		guestBase,
		backingFileName(params.Series, params.Arch()))

	out, err := params.runCmd(
		"qemu-img",
		"create",
		"-b", backingPath,
		"-f", "qcow2",
		imgPath,
		fmt.Sprintf("%dG", params.RootDisk))
	logger.Debugf("create root image: %s", out)
	if err != nil {
		return "", errors.Trace(err)
	}

	return imgPath, nil
}

// createNVRAM creates an empty NVRAM (pflash) drive. This is required for
// booting UEFI images. As of Xenial Two pflash drives are required. One
// provides the actual firmware which can be shared read-only and is provided
// in a shared directory by qemu-efi. It is in /usr/share/AAVMF/AAVMF_CODE.fd.
// The the second is for storing variables to be read on subsequent boots.
// This creates that second drive from a template.
func createNVRAM(params CreateMachineParams) error {
	if params.Arch() != arch.ARM64 {
		return nil
	}

	guestBase, err := guestPath(params.findPath)
	if err != nil {
		return errors.Trace(err)
	}
	params.nvram = filepath.Join(guestBase, fmt.Sprintf("%s-VARS.fd", params.Host()))
	f, err := os.Create(params.nvram)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		err = f.Close()
		if err != nil {
			logger.Infof("failed to close %q %s", f.Name(), err)
		}
	}()

	err = f.Truncate(64 * (1 << 20))
	if err != nil {
		return errors.Annotatef(err, "failed to create NVRAM %q", f.Name())
	}
	return nil
}

// pool info parses and returns the output of `virsh pool-info <poolname>`.
func poolInfo(runCmd runFunc) (*libvirtPool, error) {
	output, err := runCmd("virsh", "pool-info", poolName)
	if err != nil {
		logger.Debugf("pool %q doesn't appear to exist: %s", poolName, err)
		return nil, nil
	}

	p := &libvirtPool{}
	err = yaml.Unmarshal([]byte(output), p)
	if err != nil {
		logger.Errorf("failed to unmarshal info %s", err)
		return nil, errors.Trace(err)
	}
	return p, nil
}

// libvirtPool represents the guest pool information we care about.  Additional
// fields are available but ignored here.
type libvirtPool struct {
	Name      string `yaml:"Name"`
	State     string `yaml:"State"`
	Autostart string `yaml:"Autostart"`
}
