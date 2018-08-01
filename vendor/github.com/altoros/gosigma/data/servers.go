// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package data

import "io"

// ServerDrive describe properties of disk drive
type ServerDrive struct {
	BootOrder int      `json:"boot_order,omitempty"`
	Channel   string   `json:"dev_channel,omitempty"`
	Device    string   `json:"device,omitempty"`
	Drive     Resource `json:"drive,omitempty"`
}

// Server contains detail properties of cloud server instance
type Server struct {
	Resource
	Context            bool              `json:"context,omitempty"`
	CPU                uint64            `json:"cpu,omitempty"`
	CPUsInsteadOfCores bool              `json:"cpus_instead_of_cores,omitempty"`
	CPUModel           string            `json:"cpu_model,omitempty"`
	Drives             []ServerDrive     `json:"drives,omitempty"`
	Mem                uint64            `json:"mem,omitempty"`
	Meta               map[string]string `json:"meta,omitempty"`
	Name               string            `json:"name,omitempty"`
	NICs               []NIC             `json:"nics,omitempty"`
	SMP                uint64            `json:"smp,omitempty"`
	Status             string            `json:"status,omitempty"`
	VNCPassword        string            `json:"vnc_password,omitempty"`
}

// Servers holds collection of Server objects
type Servers struct {
	Meta    Meta     `json:"meta"`
	Objects []Server `json:"objects"`
}

// ReadServers reads and unmarshalls description of cloud server instances from JSON stream
func ReadServers(r io.Reader) ([]Server, error) {
	var servers Servers
	if err := ReadJSON(r, &servers); err != nil {
		return nil, err
	}
	return servers.Objects, nil
}

// ReadServer reads and unmarshalls description of single cloud server instance from JSON stream
func ReadServer(r io.Reader) (*Server, error) {
	var server Server
	if err := ReadJSON(r, &server); err != nil {
		return nil, err
	}
	return &server, nil
}
