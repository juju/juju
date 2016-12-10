// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build linux
// +build amd64 arm64 ppc64el

package libvirt

import (
	"encoding/xml"
	"fmt"

	"github.com/juju/errors"
)

// Details of the domain XML format are at: https://libvirt.org/formatdomain.html
// We only use a subset, just enough to create instances in the pool. We don't
// check any argument types here. We expect incoming params to be validate-able
// by a function on the incoming domainParams. XXX: Or validate the params before being called.

// DiskInfo is an interface to allow callers to pass DiskInfo in domainParams.
type DiskInfo interface {
	// Source is the path to the disk image.
	Source() string
	// Driver is the type of disk, qcow, vkmd, raw, etc...
	Driver() string
}

// InterfaceInfo is to allow callers to pass InterfaceInfo in domainParams.
type InterfaceInfo interface {
	MAC() string
	ParentDeviceName() string
	DeviceName() string
}

type domainParams interface {
	CPUs() uint64
	DiskInfo() []DiskInfo
	Host() string
	NetworkInfo() []InterfaceInfo
	RAM() uint64
	ValidateDomainParams() error
}

// NewDomain returns a guest domain suitable for unmarshaling (as XML) onto the
// target host.
func NewDomain(p domainParams) (Domain, error) {
	if err := p.ValidateDomainParams(); err != nil {
		return Domain{}, errors.Trace(err)
	}

	d := Domain{
		Type:     "kvm",
		OS:       OS{Type: "hvm"},
		Features: Features{},
		Disk:     []Disk{},
		Controller: []Controller{
			Controller{
				Type:  "usb",
				Index: 0,
				Address: &Address{
					Type:     "pci",
					Domain:   "0x0000",
					Bus:      "0x00",
					Slot:     "0x01",
					Function: "0x2"},
			},
			Controller{
				Type:  "pci",
				Index: 0,
				Model: "pci-root",
			},
		},
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
			Console{
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
		Input: []Input{
			Input{Type: "mouse", Bus: "ps2"},
			Input{Type: "keyboard", Bus: "ps2"},
		},
		Graphics: Graphics{
			Type:     "vnc",
			Port:     "-1",
			Autoport: "yes",
			Listen:   "127.0.0.1",
			GraphicsListen: GraphicsListen{
				Type:    "address",
				Address: "127.0.0.1",
			},
		},
		Video: Video{
			Model: Model{
				Type:  "cirrus",
				VRAM:  "9216",
				Heads: "1",
			},
			Address: Address{
				Type:     "pci",
				Domain:   "0x0000",
				Bus:      "0x00",
				Slot:     "0x02",
				Function: "0x0"},
		},
		Interface:     []Interface{},
		Name:          p.Host(),
		VCPU:          p.CPUs(),
		CurrentMemory: Memory{Unit: "MiB", Text: p.RAM()},
		Memory:        Memory{Unit: "MiB", Text: p.RAM()},
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
		d.Interface = append(d.Interface, Interface{
			Type:   "bridge",
			MAC:    InterfaceMAC{Address: iface.MAC()},
			Model:  Model{Type: "virtio"},
			Source: InterfaceSource{Bridge: iface.ParentDeviceName()},
			Guest:  InterfaceGuest{Dev: iface.DeviceName()},
		})
	}
	return d, nil
}

// deviceID generates a device id from and int. The limit of 26 is arbitrary,
// but it seems unlikely we'll need more than a couple for our use case.
func deviceID(i int) (string, error) {
	if i < 0 || i > 25 {
		return "", errors.Errorf("got %d but only support devices 0-25", i)
	}
	return fmt.Sprintf("vd%s", string('a'+i)), nil
}

// Domain describes a libvirt domain. A domain is an instance of an operating
// system running on a virtualized machine.
// See: https://libvirt.org/formatdomain.html where we only care about kvm
// specific details.
type Domain struct {
	XMLName       xml.Name     `xml:"domain"`
	Type          string       `xml:"type,attr"`
	OS            OS           `xml:"os"`
	Features      Features     `xml:"features"`
	Controller    []Controller `xml:"devices>controller"`
	Serial        Serial       `xml:"devices>serial,omitempty"`
	Console       []Console    `xml:"devices>console"`
	Input         []Input      `xml:"devices>input"`
	Graphics      Graphics     `xml:"devices>graphics"`
	Video         Video        `xml:"devices>video"`
	Interface     []Interface  `xml:"devices>interface"`
	Disk          []Disk       `xml:"devices>disk"`
	Name          string       `xml:"name"`
	VCPU          uint64       `xml:"vcpu"`
	CurrentMemory Memory       `xml:"currentMemory"`
	Memory        Memory       `xml:"memory"`
}

// OS is static. We generate a default value (kvm) for it.
// See: https://libvirt.org/formatdomain.html#elementsOSBIOS
// See also: https://libvirt.org/formatcaps.html#elementGuest
type OS struct {
	Type string `xml:"type"`
}

// Features is static. We generate empty elements for the members of this
// struct.
// See: https://libvirt.org/formatcaps.html#elementGuest
type Features struct {
	ACPI string `xml:"acpi"`
	APIC string `xml:"apic"`
	PAE  string `xml:"pae"`
}

// Controller is static. We generate a default value for it.
// See: https://libvirt.org/formatdomain.html#elementsControllers
type Controller struct {
	Type  string `xml:"type,attr"`
	Index int    `xml:"index,attr"`
	Model string `xml:"model,attr,omitempty"`
	// Address is a pointer here so we can omit an empty value.
	Address *Address `xml:"address,omitempty"`
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

// Serial is static. This was added specifially to create a functional console
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

// Input is static. We generate default values for keyboard and mouse.
// See: https://libvirt.org/formatdomain.html#elementsInput
type Input struct {
	Type string `xml:"type,attr"`
	Bus  string `xml:"bus,attr"`
}

// Graphics is static. We generate a generic vnc server by default.
// See: https://libvirt.org/formatdomain.html#elementsGraphics
type Graphics struct {
	Type           string         `xml:"type,attr"`
	Port           string         `xml:"port,attr"`
	Autoport       string         `xml:"autoport,attr"`
	Listen         string         `xml:"listen,attr"`
	GraphicsListen GraphicsListen `xml:"listen"`
}

// GraphicsListen is an element in Graphics.
// See: Graphics
type GraphicsListen struct {
	Type    string `xml:"type,attr"`
	Address string `xml:"address,attr"`
}

// Video is static. We generate a generic default value.
// See: https://libvirt.org/formatdomain.html#elementsVideo
type Video struct {
	Model   Model   `xml:"model"`
	Address Address `xml:"address"`
}

// Interface is dynamic. It represents a network interface. We generate it from
// an incoming argument.
// See: https://libvirt.org/formatdomain.html#elementsNICSBridge
type Interface struct {
	Type   string          `xml:"type,attr"`
	MAC    InterfaceMAC    `xml:"mac"`
	Model  Model           `xml:"model"`
	Source InterfaceSource `xml:"source"`
	Guest  InterfaceGuest  `xml:"guest"`
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
// behavior of libvirt. constrants.Value.Mem documents this as "megabytes".
// Interpreting that here as MiB.
// See: Memory, github.com/juju/juju/constraints/constraints.Value.Mem
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

// Model is used as an element in Video and Interface.
// See: Video, Interface
type Model struct {
	Type  string `xml:"type,attr"`
	VRAM  string `xml:"vram,attr,omitempty"`
	Heads string `xml:"heads,attr,omitempty"`
}
