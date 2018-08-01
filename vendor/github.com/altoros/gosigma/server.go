// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"fmt"
	"time"

	"github.com/altoros/gosigma/data"
)

const (
	// ServerStopped defines constant for stopped instance state
	ServerStopped = "stopped"
	// ServerStarting defines constant for starting instance state
	ServerStarting = "starting"
	// ServerRunning defines constant for running instance state
	ServerRunning = "running"
	// ServerStopping defines constant for stopping instance state
	ServerStopping = "stopping"
	// ServerUnavailable defines constant for unavailable instance state
	ServerUnavailable = "unavailable"
)

const (
	// RecurseNothing defines constant to remove server and leave all attached disks and CDROMs.
	RecurseNothing = ""
	// RecurseAllDrives defines constant to remove server and all attached drives regardless of media type they have.
	RecurseAllDrives = "all_drives"
	// RecurseDisks defines constant to remove server and all attached drives having media type "disk".
	RecurseDisks = "disks"
	// RecurseCDROMs defines constant to remove server and all attached drives having media type "cdrom".
	RecurseCDROMs = "cdroms"
)

// A Server interface represents server instance in CloudSigma account
type Server interface {
	// CloudSigma resource
	Resource

	// Context serial device enabled for server instance
	Context() bool

	// Cpu frequency in MHz
	CPU() uint64

	// Selects whether the SMP is exposed as cores of a single CPU or separate CPUs.
	// This should be set to false for Windows, because there are license
	// requirements for multiple CPUs.
	CPUsInsteadOfCores() bool

	// Virtual CPU model, for mitigating compatibility issues between the guest operating system
	// and the underlying host's CPU. If not specified, all of the hypervisor's CPUs
	// capabilities are passed directly to the virtual machine.
	CPUModel() string

	// Drives for this server instance
	Drives() []ServerDrive

	// Mem capacity in bytes
	Mem() uint64

	// Name of server instance
	Name() string

	// NICs for this server instance
	NICs() []NIC

	// Symmetric Multiprocessing (SMP) i.e. number of CPU cores
	SMP() uint64

	// Status of server instance
	Status() string

	// VNCPassword to access the server
	VNCPassword() string

	// Get meta-information value stored in the server instance
	Get(key string) (string, bool)

	// Refresh information about server instance
	Refresh() error

	// Start server instance. This method does not check current server status,
	// start command is issued to the endpoint in case of any value cached in Status().
	Start() error

	// Stop server instance. This method does not check current server status,
	// stop command is issued to the endpoint in case of any value cached in Status().
	Stop() error

	// Start server instance and waits for status ServerRunning with timeout
	StartWait() error

	// Stop server instance and waits for status ServerStopped with timeout
	StopWait() error

	// Remove server instance
	Remove(recurse string) error

	// Wait for user-defined event
	Wait(stop func(Server) bool) error

	// IPv4 finds all assigned IPv4 addresses at runtime
	IPv4() []string
}

// A server implements server instance in CloudSigma account
type server struct {
	client *Client
	obj    *data.Server
}

var _ Server = (*server)(nil)

// String method implements fmt.Stringer interface
func (s server) String() string {
	return fmt.Sprintf("{Name: %q\nURI: %q\nStatus: %s\nUUID: %q}",
		s.Name(), s.URI(), s.Status(), s.UUID())
}

// URI of server instance
func (s server) URI() string { return s.obj.URI }

// UUID of server instance
func (s server) UUID() string { return s.obj.UUID }

// Context serial device enabled for server instance
func (s server) Context() bool { return s.obj.Context }

// Cpu frequency in MHz
func (s server) CPU() uint64 { return s.obj.CPU }

// Selects whether the SMP is exposed as cores of a single CPU or separate CPUs.
// This should be set to false for Windows, because there are license
// requirements for multiple CPUs.
func (s server) CPUsInsteadOfCores() bool { return s.obj.CPUsInsteadOfCores }

// Virtual CPU model, for mitigating compatibility issues between the guest operating system
// and the underlying host's CPU. If not specified, all of the hypervisor's CPUs
// capabilities are passed directly to the virtual machine.
func (s server) CPUModel() string { return s.obj.CPUModel }

// Drives for this server instance
func (s server) Drives() []ServerDrive {
	r := make([]ServerDrive, 0, len(s.obj.Drives))
	for i := range s.obj.Drives {
		drive := &serverDrive{s.client, &s.obj.Drives[i]}
		r = append(r, drive)
	}
	return r
}

// Mem capacity in bytes
func (s server) Mem() uint64 { return s.obj.Mem }

// Name of server instance
func (s server) Name() string { return s.obj.Name }

// NICs for this server instance
func (s server) NICs() []NIC {
	r := make([]NIC, 0, len(s.obj.NICs))
	for i := range s.obj.NICs {
		n := nic{s.client, &s.obj.NICs[i]}
		r = append(r, n)
	}
	return r
}

// Symmetric Multiprocessing (SMP) i.e. number of CPU cores
func (s server) SMP() uint64 { return s.obj.SMP }

// Status of server instance
func (s server) Status() string { return s.obj.Status }

// VNCPassword to access the server
func (s server) VNCPassword() string { return s.obj.VNCPassword }

// Get meta-information value stored in the server instance
func (s server) Get(key string) (v string, ok bool) {
	v, ok = s.obj.Meta[key]
	return
}

// Refresh information about server instance
func (s *server) Refresh() error {
	obj, err := s.client.getServer(s.UUID())
	if err != nil {
		return err
	}
	s.obj = obj
	return nil
}

// Start server instance. This method does not check current server status,
// start command is issued to the endpoint in case of any value cached in Status().
func (s server) Start() error {
	return s.client.startServer(s.UUID(), nil)
}

// Stop server instance. This method does not check current server status,
// stop command is issued to the endpoint in case of any value cached in Status().
func (s server) Stop() error {
	return s.client.stopServer(s.UUID())
}

// Start server instance and waits for status ServerRunning with timeout
func (s *server) StartWait() error {
	if err := s.Start(); err != nil {
		return err
	}
	return s.Wait(func(srv Server) bool {
		return srv.Status() == ServerRunning
	})
}

// Stop server instance and waits for status ServerStopped with timeout
func (s *server) StopWait() error {
	if err := s.Stop(); err != nil {
		return err
	}
	return s.Wait(func(srv Server) bool {
		return srv.Status() == ServerStopped
	})
}

// Remove server instance
func (s server) Remove(recurse string) error {
	return s.client.removeServer(s.UUID(), recurse)
}

// Wait for user-defined event
func (s *server) Wait(stop func(srv Server) bool) error {
	var timedout = false

	timeout := s.client.GetOperationTimeout()
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { timedout = true })
		defer timer.Stop()
	}

	for !stop(s) {
		if err := s.Refresh(); err != nil {
			return err
		}
		if timedout {
			return ErrOperationTimeout
		}
	}

	return nil
}

// IPv4 finds all assigned IPv4 addresses at runtime
func (s server) IPv4() []string {
	var result []string
	for _, n := range s.obj.NICs {
		if n.Runtime == nil {
			continue
		}
		if n.Runtime.IPv4 == nil {
			continue
		}
		if n.Runtime.IPv4.UUID != "" {
			result = append(result, n.Runtime.IPv4.UUID)
		}
	}
	return result
}
