// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"strings"

	"github.com/altoros/gosigma/data"
)

const (
	// Kilobyte defines constant for specifying kilobytes
	Kilobyte = 1024
	// Megabyte defines constant for specifying megabytes
	Megabyte = 1024 * 1024
	// Gigabyte defines constant for specifying gigabytes
	Gigabyte = 1024 * 1024 * 1024
	// Terabyte defines constant for specifying terabytes
	Terabyte = 1024 * 1024 * 1024 * 1024
)

const (
	// ModelVirtio defines constant for "virtio" driver model
	ModelVirtio = "virtio"
	// ModelE1000 defines constant for "e1000" driver model
	ModelE1000 = "e1000"
)

// A Components contains information to create new server
type Components struct {
	data *data.Server
}

// SetName sets name for new server. To unset name, call this function with empty string in the name parameter.
func (c *Components) SetName(name string) {
	c.init()
	c.data.Name = strings.TrimSpace(name)
}

// SetCPU sets CPU frequency for new server. To unset CPU frequency, call this function with zero in the frequency parameter.
func (c *Components) SetCPU(frequency uint64) {
	c.init()
	c.data.CPU = frequency
}

// SetSMP sets number of CPU cores for new server. To unset CPU cores, call this function with zero in the cores parameter.
func (c *Components) SetSMP(cores uint64) {
	c.init()
	c.data.SMP = cores
}

// SetMem sets memory size for new server. To unset this value, call function with zero in the bytes parameter.
func (c *Components) SetMem(bytes uint64) {
	c.init()
	c.data.Mem = bytes
}

// SetVNCPassword sets VNC password for new server. To unset, call this function with empty string.
func (c *Components) SetVNCPassword(password string) {
	c.init()
	c.data.VNCPassword = strings.TrimSpace(password)
}

// SetMeta information for new server
func (c *Components) SetMeta(name, value string) {
	c.init()

	m := c.data.Meta

	value = strings.TrimSpace(value)
	if value == "" {
		delete(m, name)
	} else {
		m[name] = value
	}
}

// SetDescription sets description for new server. To unset, call this function with empty string.
func (c *Components) SetDescription(description string) {
	c.SetMeta("description", description)
}

// SetSSHPublicKey sets public SSH key for new server. To unset, call this function with empty string.
func (c *Components) SetSSHPublicKey(description string) {
	c.SetMeta("ssh_public_key", description)
}

// AttachDrive attaches drive to components from drive data.
func (c *Components) AttachDrive(bootOrder int, channel, device, uuid string) {
	c.init()

	var sd data.ServerDrive
	sd.BootOrder = bootOrder
	sd.Channel = strings.TrimSpace(channel)
	sd.Device = strings.TrimSpace(device)
	sd.Drive = *data.MakeDriveResource(uuid)

	c.data.Drives = append(c.data.Drives, sd)
}

// NetworkDHCP4 attaches NIC, configured with IPv4 DHCP
func (c *Components) NetworkDHCP4(model string) {
	c.network4(model, "dhcp", "")
}

// NetworkStatic4 attaches NIC, configured with IPv4 static address
func (c *Components) NetworkStatic4(model, address string) {
	c.network4(model, "static", address)
}

// NetworkManual4 attaches NIC, configured with IPv4 manual settings
func (c *Components) NetworkManual4(model string) {
	c.network4(model, "manual", "")
}

// NetworkVLan attaches NIC, configured with private VLan
func (c *Components) NetworkVLan(model, uuid string) {
	c.init()

	var n data.NIC

	n.Model = strings.TrimSpace(model)
	n.VLAN = data.MakeVLanResource(uuid)

	c.data.NICs = append(c.data.NICs, n)
}

func (c *Components) network4(model, conf, address string) {
	c.init()

	var n data.NIC

	n.Model = strings.TrimSpace(model)
	n.IPv4 = &data.IPv4{Conf: conf}
	if address = strings.TrimSpace(address); address != "" {
		n.IPv4.IP = data.MakeIPResource(address)
	}

	c.data.NICs = append(c.data.NICs, n)
}

func (c *Components) init() {
	if c.data == nil {
		c.data = &data.Server{
			Meta: make(map[string]string),
		}
	}
}

func (c Components) marshal() (io.Reader, error) {
	if c.data == nil {
		return strings.NewReader("{}"), nil
	}
	bb, err := json.Marshal(c.data)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(bb), nil
}

func (c Components) marshalString() (string, error) {
	r, err := c.marshal()
	if err != nil {
		return "", err
	}
	bb, err := ioutil.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(bb), nil
}
