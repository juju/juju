// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt

import (
	"encoding/xml"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
)

// Details of the domain XML format are at: https://libvirt.org/formatdomain.html
// We only use a subset, just enough to create instances in the pool. We don't
// check any argument types here. We expect incoming params to be validate-able
// by a function on the incoming domainParams.

// DiskInfo represents the type and location of a libvirt pool image.
type DiskInfo interface {
	// Source is the path to the disk image.
	Source() string
	// Driver is the type of disk, qcow, vkmd, raw, etc...
	Driver() string
}

// InterfaceInfo represents network interface parameters for a kvm domain.
type InterfaceInfo interface {
	// MAC returns the interfaces MAC address.
	MACAddress() string
	// ParentInterfaceName returns the interface's parent device name.
	ParentInterfaceName() string
	// ParentVirtualPortType returns the type of the virtual port for this
	// interface's parent (e.g. for bridging to an OVS-managed device) or
	// an empty value if no virtual port is used.
	ParentVirtualPortType() string
	// InterfaceName returns the interface's device name.
	InterfaceName() string
}

type domainParams interface {
	// Arch returns the arch for which we want to generate the domainXML.
	Arch() string
	// CPUs returns the number of CPUs to use.
	CPUs() uint64
	// DiskInfo returns the disk information for the domain.
	DiskInfo() []DiskInfo
	// Host returns the host name.
	Host() string
	// Loader returns the path to the EFI firmware blob to UEFI boot into an
	// image. This is a read-only "pflash" drive.
	Loader() string
	// NetworkInfo contains the network interfaces to create in the domain.
	NetworkInfo() []InterfaceInfo
	// RAM returns the amount of RAM to use.
	RAM() uint64
	// ValidateDomainParams returns nil if the domainParams are valid.
	ValidateDomainParams() error
}

// NewDomain returns a guest domain suitable for unmarshaling (as XML) onto the
// target host.
func NewDomain(p domainParams) (Domain, error) {
	if err := p.ValidateDomainParams(); err != nil {
		return Domain{}, errors.Trace(err)
	}

	d := Domain{
		Type:          "kvm",
		Name:          p.Host(),
		VCPU:          p.CPUs(),
		CurrentMemory: Memory{Unit: "MiB", Text: p.RAM()},
		Memory:        Memory{Unit: "MiB", Text: p.RAM()},
		OS:            generateOSElement(p),
		Features:      generateFeaturesElement(p),
		CPU:           generateCPU(p),
		Disk:          []Disk{},
		Interface:     []Interface{},
		Serial: Serial{
			Type: "pty",
			Source: SerialSource{
				Path: "/dev/pts/2",
			},
			Target: SerialTarget{
				Port: 0,
			},
		},
		Console: []Console{
			{
				Type: "pty",
				TTY:  "/dev/pts/2",
				Target: ConsoleTarget{
					Port: 0,
				},
				Source: ConsoleSource{
					Path: "/dev/pts/2",
				},
			},
		},
	}
	for i, diskInfo := range p.DiskInfo() {
		devID, err := deviceID(i)
		if err != nil {
			return Domain{}, errors.Trace(err)
		}
		switch diskInfo.Driver() {
		case "raw":
			d.Disk = append(d.Disk, Disk{
				Device: "disk",
				Type:   "file",
				Driver: DiskDriver{Type: diskInfo.Driver(), Name: "qemu"},
				Source: DiskSource{File: diskInfo.Source()},
				Target: DiskTarget{Dev: devID},
			})
		case "qcow2":
			d.Disk = append(d.Disk, Disk{
				Device: "disk",
				Type:   "file",
				Driver: DiskDriver{Type: diskInfo.Driver(), Name: "qemu"},
				Source: DiskSource{File: diskInfo.Source()},
				Target: DiskTarget{Dev: devID},
			})
		default:
			return Domain{}, errors.Errorf(
				"unsupported disk type %q", diskInfo.Driver())
		}
	}
	for _, iface := range p.NetworkInfo() {
		virtIf := Interface{
			Type:   "bridge",
			MAC:    InterfaceMAC{Address: iface.MACAddress()},
			Model:  Model{Type: "virtio"},
			Source: InterfaceSource{Bridge: iface.ParentInterfaceName()},
			Guest:  InterfaceGuest{Dev: iface.InterfaceName()},
		}
		if vpType := iface.ParentVirtualPortType(); vpType != "" {
			virtIf.VirtualPortType = &InterfaceVirtualPort{
				Type: vpType,
			}
		}
		d.Interface = append(d.Interface, virtIf)
	}
	return d, nil
}

// generateOSElement creates the architecture appropriate element details.
func generateOSElement(p domainParams) OS {
	switch p.Arch() {
	case arch.ARM64:
		return OS{
			Type: OSType{
				Arch:    "aarch64",
				Machine: "virt",
				Text:    "hvm",
			},

			Loader: &NVRAMCode{
				Text:     p.Loader(),
				ReadOnly: "yes",
				Type:     "pflash",
			},
		}
	default:
		return OS{Type: OSType{Text: "hvm"}}
	}
}

// generateFeaturesElement generates the appropriate features element based on
// the architecture.
func generateFeaturesElement(p domainParams) *Features {
	f := new(Features)
	if p.Arch() == arch.ARM64 {
		f.GIC = &GIC{Version: "host"}
	}
	return f
}

// generateCPU infor generates any model/fallback related settings. These are
// typically to allow for better compatibility across versions of libvirt/qemu AFAIU.
func generateCPU(p domainParams) *CPU {
	return &CPU{
		Mode:  "host-passthrough",
		Check: "none",
	}
}

// deviceID generates a device id from and int. The limit of 26 is arbitrary,
// but it seems unlikely we'll need more than a couple for our use case.
func deviceID(i int) (string, error) {
	if i < 0 || i > 25 {
		return "", errors.Errorf("got %d but only support devices 0-25", i)
	}
	return fmt.Sprintf("vd%s", string(rune('a'+i))), nil
}

// Domain describes a libvirt domain. A domain is an instance of an operating
// system running on a virtualized machine.
// See: https://libvirt.org/formatdomain.html where we only care about kvm
// specific details.
type Domain struct {
	XMLName       xml.Name    `xml:"domain"`
	Type          string      `xml:"type,attr"`
	Name          string      `xml:"name"`
	VCPU          uint64      `xml:"vcpu"`
	CurrentMemory Memory      `xml:"currentMemory"`
	Memory        Memory      `xml:"memory"`
	OS            OS          `xml:"os"`
	Features      *Features   `xml:"features,omitempty"`
	CPU           *CPU        `xml:"cpu,omitempty"`
	Disk          []Disk      `xml:"devices>disk"`
	Interface     []Interface `xml:"devices>interface"`
	Serial        Serial      `xml:"devices>serial,omitempty"`
	Console       []Console   `xml:"devices>console"`
}

// OS is static. We generate a default value (kvm) for it.
// See: https://libvirt.org/formatdomain.html#elementsOSBIOS
// See also: https://libvirt.org/formatcaps.html#elementGuest
type OS struct {
	Type OSType `xml:"type"`
	// Loader is a pointer so it is omitted if empty.
	Loader *NVRAMCode `xml:"loader,omitempty"`
}

// OSType provides details that are required on certain architectures, e.g.
// ARM64.
// See: https://libvirt.org/formatdomain.html#elementsOS
type OSType struct {
	Text    string `xml:",chardata"`
	Arch    string `xml:"arch,attr,omitempty"`
	Machine string `xml:"machine,attr,omitempty"`
}

// NVRAMCode represents the "firmware blob". In our case that is the UEFI code
// which is of type pflash.
// See: https://libvirt.org/formatdomain.html#elementsOS
type NVRAMCode struct {
	Text     string `xml:",chardata"`
	ReadOnly string `xml:"readonly,attr,omitempty"`
	Type     string `xml:"type,attr,omitempty"`
}

// Features allows us to request one or more hypervisor features to be toggled
// on/off. See: https://libvirt.org/formatdomain.html#elementsFeatures
type Features struct {
	GIC  *GIC   `xml:"gic,omitempty"`
	ACPI string `xml:"acpi"`
}

// GIC is the Generic Interrupt Controller and is required to UEFI boot on
// ARM64.
//
// NB: Dann Frazier (irc:dannf) reports:
// To deploy trusty, we'll either need to use a GICv2 host, or use the HWE
// kernel in your guest. There are no official cloud images w/ HWE kernel
// preinstalled AFAIK.
// The systems we have in our #hyperscale lab are GICv3 (requiring an HWE
// kernel) - but the system Juju QA has had for a while (McDivitt) is GICv2, so
// it should be able to boot a standard trusty EFI cloud image.  Either way,
// you'll need a xenial *host*, at least to have a new enough version of
// qemu-efi and so libvirt can parse the gic_version=host xml.
//
// TODO(ro) 2017-01-20 Determine if we can provide details to reliably boot
// trusty, or if we should exit on error if we are trying to boot trusty on
// arm64.
//
// See: https://libvirt.org/formatdomain.html#elementsFeatures
type GIC struct {
	Version string `xml:"version,attr,omitempty"`
}

// CPU defines CPU topology and model requirements.
// See: https://libvirt.org/formatdomain.html#elementsCPU
type CPU struct {
	Mode  string `xml:"mode,attr,omitempty"`
	Match string `xml:"match,attr,omitempty"`
	Check string `xml:"check,attr,omitempty"`
}

// Address is static. We generate a default value for it.
// See: Controller, Video
type Address struct {
	Type     string `xml:"type,attr,omitepmty"`
	Domain   string `xml:"domain,attr,omitempty"`
	Bus      string `xml:"bus,attr,omitempty"`
	Slot     string `xml:"slot,attr,omitempty"`
	Function string `xml:"function,attr,omitempty"`
}

// Console is static. We generate a default value for it.
// See: https://libvirt.org/formatdomain.html#elementsConsole
type Console struct {
	Type   string        `xml:"type,attr"`
	TTY    string        `xml:"tty,attr,omitempty"`
	Source ConsoleSource `xml:"source,omitempty"`
	Target ConsoleTarget `xml:"target,omitempty"`
}

// ConsoleTarget is static. We generate a default value for it.
// See: Console
type ConsoleTarget struct {
	Type string `xml:"type,attr,omitempty"`
	Port int    `xml:"port,attr"`
	Path string `xml:"path,attr,omitempty"`
}

// ConsoleSource is static. We generate a default value for it.
// See: Console
type ConsoleSource struct {
	Path string `xml:"path,attr,omitempty"`
}

// Serial is static. This was added specifically to create a functional console
// for troubleshooting vms as they boot. You can attach to this console with
// `virsh console <domainName>`.
// See: https://libvirt.org/formatdomain.html#elementsConsole
type Serial struct {
	Type   string       `xml:"type,attr"`
	Source SerialSource `xml:"source"`
	Target SerialTarget `xml:"target"`
}

// SerialSource is static. We generate a default value for it.
// See: Serial
type SerialSource struct {
	Path string `xml:"path,attr"`
}

// SerialTarget is static. We generate a default value for it.
// See: Serial
type SerialTarget struct {
	Port int `xml:"port,attr"`
}

// Interface is dynamic. It represents a network interface. We generate it from
// an incoming argument.
// See: https://libvirt.org/formatdomain.html#elementsNICSBridge
type Interface struct {
	Type            string                `xml:"type,attr"`
	MAC             InterfaceMAC          `xml:"mac"`
	Model           Model                 `xml:"model"`
	Source          InterfaceSource       `xml:"source"`
	Guest           InterfaceGuest        `xml:"guest"`
	VirtualPortType *InterfaceVirtualPort `xml:"virtualport,omitempty"`
}

// InterfaceVirtualPort provides additional configuration data to be forwarded
// to a vepa (802.1Qbg) or 802.1Qbh compliant switch, or to an Open vSwitch
// virtual switch.
type InterfaceVirtualPort struct {
	Type string `xml:"type,attr"`
}

// InterfaceMAC is the MAC address for an Interface.
// See: Interface
type InterfaceMAC struct {
	Address string `xml:"address,attr"`
}

// InterfaceSource it the host bridge to the network.
// See: Interface
type InterfaceSource struct {
	Bridge string `xml:"bridge,attr"`
}

// InterfaceGuest is the guests network device.
// See: Interface
type InterfaceGuest struct {
	Dev string `xml:"dev,attr"`
}

// Disk is dynamic. We create it with paths to the user data source and disk.
// See: https://libvirt.org/formatdomain.html#elementsDisks
type Disk struct {
	Device string     `xml:"device,attr"`
	Type   string     `xml:"type,attr"`
	Driver DiskDriver `xml:"driver"`
	Source DiskSource `xml:"source"`
	Target DiskTarget `xml:"target"`
}

// DiskDriver is the type of virtual disk. We generate it dynamically.
// See: Disk
type DiskDriver struct {
	Type string `xml:"type,attr"`
	Name string `xml:"name,attr"`
}

// DiskSource is the location of the disk image. In our case the path to the
// necessary images.
// See: Disk
type DiskSource struct {
	File string `xml:"file,attr"`
}

// DiskTarget is the target device on the guest. We generate these.
type DiskTarget struct {
	Dev string `xml:"dev,attr"`
}

// CurrentMemory is the actual allocation of memory for the guest. It appears
// we historically set this the same as Memory, which is also the default
// behavior of libvirt. Constraints.Value.Mem documents this as "megabytes".
// Interpreting that here as MiB.
// See: Memory, github.com/juju/juju/core/constraints/constraints.Value.Mem
type CurrentMemory struct {
	Unit string `xml:"unit,attr"`
	Text uint64 `xml:",chardata"`
}

// Memory is dynamic. We take an argument to set it. Unit is magnitude of
// memory: b, k or KiB, KB, M or MiB, MB, etc... The libvirt default is KiB. We want to
// set MiB so we default to that.
// See: https://libvirt.org/formatdomain.html#elementsMemoryAllocation
type Memory struct {
	Unit string `xml:"unit,attr,omitempty"`
	Text uint64 `xml:",chardata"`
}

// Model is used as an element in CPU and Interface.
// See: CPU, Interface
type Model struct {
	Fallback string `xml:"fallback,attr,omitempty"`
	Text     string `xml:",chardata"`
	Type     string `xml:"type,attr,omitempty"`
}
