// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import "io"

// ContextDrive describes properties of disk drive in server context
type ContextDrive struct {
	BootOrder int    `json:"boot_order,omitempty"`
	Channel   string `json:"dev_channel,omitempty"`
	Device    string `json:"device,omitempty"`
}

// ContextIPv4Config describes IPv4 parameters in server context
type ContextIPv4Config struct {
	Gateway     string            `json:"gateway"`
	Meta        map[string]string `json:"meta"`
	Nameservers []string          `json:"nameservers"`
	Netmask     int               `json:"netmask"`
	UUID        string            `json:"uuid"`
}

// ContextIPv4 describes IPv4 configuration in server context
type ContextIPv4 struct {
	Conf string            `json:"conf"`
	IP   ContextIPv4Config `json:"ip"`
}

// ContextVLan describes network interface properties for VLan in server context
type ContextVLan struct {
	Meta map[string]string `json:"meta,omitempty"`
	Tags []string          `json:"tags,omitempty"`
	UUID string            `json:"uuid"`
}

// ContextNIC describes network interface properties in server context
type ContextNIC struct {
	IPv4  *ContextIPv4 `json:"ip_v4_conf"`
	Mac   string       `json:"mac"`
	Model string       `json:"model"`
	VLan  *ContextVLan `json:"vlan"`
}

// Context contains detail properties of server instance context
type Context struct {
	CPU         int64             `json:"cpu,omitempty"`
	CPUModel    string            `json:"cpu_model,omitempty"`
	Drives      []ContextDrive    `json:"drives,omitempty"`
	Mem         int64             `json:"mem,omitempty"`
	Meta        map[string]string `json:"meta,omitempty"`
	Name        string            `json:"name,omitempty"`
	NICs        []ContextNIC      `json:"nics,omitempty"`
	UUID        string            `json:"uuid,omitempty"`
	VNCPassword string            `json:"vnc_password,omitempty"`
}

// ReadContext reads and unmarshalls server instance context from JSON stream
func ReadContext(r io.Reader) (*Context, error) {
	var c Context
	if err := ReadJSON(r, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
