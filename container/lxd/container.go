// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/v2/arch"
	"github.com/lxc/lxd/shared/api"
	"github.com/lxc/lxd/shared/units"
	"github.com/lxc/lxd/shared/version"

	"github.com/juju/juju/core/constraints"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

const (
	UserNamespacePrefix = "user."
	UserDataKey         = UserNamespacePrefix + "user-data"
	NetworkConfigKey    = UserNamespacePrefix + "network-config"
	JujuModelKey        = UserNamespacePrefix + "juju-model"
	AutoStartKey        = "boot.autostart"
)

// ContainerSpec represents the data required to create a new container.
type ContainerSpec struct {
	Architecture string
	Name         string
	Image        SourcedImage
	Devices      map[string]device
	Config       map[string]string
	Profiles     []string
	InstanceType string
}

// minMiBVersion is the minimum LXD version that we are sure will recognise the
// MiB suffix for memory constraints. By default we use "MB".
var minMiBVersion = &version.DottedVersion{Major: 3, Minor: 10}

// ApplyConstraints applies the input constraints as valid LXD container
// configuration to the container spec.
// Note that we pass these through as supplied. If an instance type constraint
// has been specified along with specific cores/mem constraints,
// LXD behaviour is to override with the specific ones even when lower.
func (c *ContainerSpec) ApplyConstraints(serverVersion string, cons constraints.Value) error {
	if cons.HasInstanceType() {
		c.InstanceType = *cons.InstanceType
	}
	if cons.HasCpuCores() {
		c.Config["limits.cpu"] = fmt.Sprintf("%d", *cons.CpuCores)
	}
	if cons.HasMem() {
		// Ensure that we use the correct "MB"/"MiB" suffix.
		template := "%dMB"
		if current, err := version.Parse(serverVersion); err == nil {
			if current.Compare(minMiBVersion) >= 0 {
				template = "%dMiB"
			}
		}
		c.Config["limits.memory"] = fmt.Sprintf(template, *cons.Mem)
	}
	if cons.HasArch() {
		c.Architecture = *cons.Arch
	}

	return nil
}

// Container extends the upstream LXD container type.
type Container struct {
	api.Container
}

// Metadata returns the value from container config for the input key.
// Such values are stored with the "user" namespace prefix.
func (c *Container) Metadata(key string) string {
	return c.Config[UserNamespacePrefix+key]
}

// Arch returns the architecture of the container.
func (c *Container) Arch() string {
	return arch.NormaliseArch(c.Architecture)
}

// CPUs returns the configured limit for number of container CPU cores.
// If unset, zero is returned.
func (c *Container) CPUs() uint {
	var cores uint
	if v := c.Config["limits.cpu"]; v != "" {
		_, err := fmt.Sscanf(v, "%d", &cores)
		if err != nil {
			logger.Errorf("failed to parse %q into uint, ignoring err: %s", v, err)
		}
	}
	return cores
}

// Mem returns the configured limit for container memory in MiB.
func (c *Container) Mem() uint {
	v := c.Config["limits.memory"]
	if v == "" {
		return 0
	}

	bytes, err := units.ParseByteSizeString(v)
	if err != nil {
		logger.Errorf("failed to parse %q into bytes, ignoring err: %s", v, err)
		return 0
	}

	const oneMiB = 1024 * 1024
	mib := bytes / oneMiB
	if mib > math.MaxUint32 {
		logger.Errorf("byte string %q overflowed uint32, using max value", v)
		return math.MaxUint32
	}

	return uint(mib)
}

// AddDisk modifies updates the container's devices map to represent a disk
// device described by the input arguments.
// If the device already exists, an error is returned.
func (c *Container) AddDisk(name, path, source, pool string, readOnly bool) error {
	if _, ok := c.Devices[name]; ok {
		return errors.Errorf("container %q already has a device %q", c.Name, name)
	}

	if c.Devices == nil {
		c.Devices = map[string]device{}
	}
	c.Devices[name] = map[string]string{
		"path":   path,
		"source": source,
		"type":   "disk",
	}
	if pool != "" {
		c.Devices[name]["pool"] = pool
	}
	if readOnly {
		c.Devices[name]["readonly"] = "true"
	}
	return nil
}

// aliveStatuses is the list of status strings that indicate
// a container is "alive".
var aliveStatuses = []string{
	api.Starting.String(),
	api.Started.String(),
	api.Running.String(),
	api.Stopping.String(),
	api.Stopped.String(),
}

// AliveContainers returns the list of containers based on the input namespace
// prefixed that are in a status indicating they are "alive".
func (s *Server) AliveContainers(prefix string) ([]Container, error) {
	c, err := s.FilterContainers(prefix, aliveStatuses...)
	return c, errors.Trace(err)
}

// FilterContainers retrieves the list of containers from the server and filters
// them based on the input namespace prefix and any supplied statuses.
func (s *Server) FilterContainers(prefix string, statuses ...string) ([]Container, error) {
	containers, err := s.GetContainers()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []Container
	for _, c := range containers {
		if prefix != "" && !strings.HasPrefix(c.Name, prefix) {
			continue
		}
		if len(statuses) > 0 && !containerHasStatus(c, statuses) {
			continue
		}
		results = append(results, Container{c})
	}
	return results, nil
}

// ContainerAddresses gets usable network addresses for the container
// identified by the input name.
func (s *Server) ContainerAddresses(name string) ([]corenetwork.ProviderAddress, error) {
	state, _, err := s.GetContainerState(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	networks := state.Network
	if networks == nil {
		return []corenetwork.ProviderAddress{}, nil
	}

	var results []corenetwork.ProviderAddress
	for netName, net := range networks {
		if netName == network.DefaultLXCBridge || netName == network.DefaultLXDBridge {
			continue
		}
		for _, addr := range net.Addresses {
			netAddr := corenetwork.NewProviderAddress(addr.Address)
			if netAddr.Scope == corenetwork.ScopeLinkLocal || netAddr.Scope == corenetwork.ScopeMachineLocal {
				logger.Tracef("ignoring address %q for container %q", addr, name)
				continue
			}
			results = append(results, netAddr)
		}
	}
	return results, nil
}

// CreateContainerFromSpec creates a new container based on the input spec,
// and starts it immediately.
// If the container fails to be started, it is removed.
// Upon successful creation and start, the container is returned.
func (s *Server) CreateContainerFromSpec(spec ContainerSpec) (*Container, error) {
	logger.Infof("starting new container %q (image %q)", spec.Name, spec.Image.Image.Filename)
	logger.Debugf("new container has profiles %v", spec.Profiles)
	req := api.ContainersPost{
		Name:         spec.Name,
		InstanceType: spec.InstanceType,
		ContainerPut: api.ContainerPut{
			Architecture: spec.Architecture,
			Profiles:     spec.Profiles,
			Devices:      spec.Devices,
			Config:       spec.Config,
			Ephemeral:    false,
		},
	}
	op, err := s.CreateContainerFromImage(spec.Image.LXDServer, *spec.Image.Image, req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := op.Wait(); err != nil {
		return nil, errors.Trace(err)
	}
	opInfo, err := op.GetTarget()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if opInfo.StatusCode != api.Success {
		return nil, fmt.Errorf("container creation failed: %s", opInfo.Err)
	}

	logger.Debugf("created container %q, waiting for start...", spec.Name)

	if err := s.StartContainer(spec.Name); err != nil {
		if remErr := s.RemoveContainer(spec.Name); remErr != nil {
			logger.Errorf("failed to remove container after unsuccessful start: %s", remErr.Error())
		}
		return nil, errors.Trace(err)
	}

	container, _, err := s.GetContainer(spec.Name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := Container{*container}
	return &c, nil
}

// StartContainer starts the extant container identified by the input name.
func (s *Server) StartContainer(name string) error {
	req := api.ContainerStatePut{
		Action:   "start",
		Timeout:  -1,
		Force:    false,
		Stateful: false,
	}
	op, err := s.UpdateContainerState(name, req, "")
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(op.Wait())
}

// Remove containers stops and deletes containers matching the input list of
// names. Any failed removals are indicated in the returned error.
func (s *Server) RemoveContainers(names []string) error {
	if len(names) == 0 {
		return nil
	}

	var failed []string
	for _, name := range names {
		if err := s.RemoveContainer(name); err != nil {
			failed = append(failed, name)
			logger.Errorf("removing container %q: %v", name, err)
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("failed to remove containers: %s", strings.Join(failed, ", "))
	}
	return nil
}

// Remove container first ensures that the container is stopped,
// then deletes it.
func (s *Server) RemoveContainer(name string) error {
	state, eTag, err := s.GetContainerState(name)
	if err != nil {
		return errors.Trace(err)
	}

	if state.StatusCode != api.Stopped {
		req := api.ContainerStatePut{
			Action:   "stop",
			Timeout:  -1,
			Force:    true,
			Stateful: false,
		}
		op, err := s.UpdateContainerState(name, req, eTag)
		if err != nil {
			return errors.Trace(err)
		}
		if err := op.Wait(); err != nil {
			return errors.Trace(err)
		}
	}

	// NOTE(achilleasa): the (apt) lxd version that ships with bionic
	// does not automatically remove veth devices if attached to an OVS
	// bridge. The operator must manually remove these devices from the
	// bridge by running "ovs-vsctl --if-exists del-port X". This issue
	// has been fixed in newer lxd versions.

	// LXD has issues deleting containers, even if they've been stopped. The
	// general advice passed back from the LXD team is to retry it again, to
	// see if this helps clean up the containers.
	// ZFS exacerbates this more for the LXD setup, but there is no way to
	// know as the LXD client doesn't return typed errors.
	retryArgs := retry.CallArgs{
		Clock: s.Clock(),
		IsFatalError: func(err error) bool {
			return errors.IsBadRequest(err)
		},
		Func: func() error {
			op, err := s.DeleteContainer(name)
			if err != nil {
				// sigh, LXD not found container - it's been deleted so, we
				// just need to return nil.
				if IsLXDNotFound(errors.Cause(err)) {
					return nil
				}
				return errors.BadRequestf(err.Error())
			}
			return errors.Trace(op.Wait())
		},
		Delay:    2 * time.Second,
		Attempts: 3,
	}
	if err := retry.Call(retryArgs); err != nil {
		return errors.Trace(errors.Cause(err))
	}
	return nil
}

// WriteContainer writes the current representation of the input container to
// the server.
func (s *Server) WriteContainer(c *Container) error {
	resp, err := s.UpdateContainer(c.Name, c.Writable(), "")
	if err != nil {
		return errors.Trace(err)
	}
	if err := resp.Wait(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (s *Server) Clock() clock.Clock {
	if s.clock == nil {
		return clock.WallClock
	}
	return s.clock
}

// containerHasStatus returns true if the input container has a status
// matching one from the input list.
func containerHasStatus(container api.Container, statuses []string) bool {
	for _, status := range statuses {
		if container.StatusCode.String() == status {
			return true
		}
	}
	return false
}
