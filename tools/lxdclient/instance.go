// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/juju/juju/container/lxd"
	"github.com/juju/utils/arch"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
)

// Constants related to user metadata.
const (
	MetadataNamespace = "user"

	// This is defined by the cloud-init code:
	// http://bazaar.launchpad.net/~cloud-init-dev/cloud-init/trunk/view/head:/cloudinit/sources/
	// http://cloudinit.readthedocs.org/en/latest/
	// Also see https://github.com/lxc/lxd/blob/master/specs/configuration.md.
	UserdataKey      = "user-data"
	NetworkconfigKey = "network-config"

	// CertificateFingerprintKey is a key that we define to associate
	// a certificate fingerprint with an instance. We use this to clean
	// up certificates when removing controller instances.
	CertificateFingerprintKey = "certificate-fingerprint"

	megabyte = 1024 * 1024
)

// ResolveConfigKey applies the specified namespaces to the config key
// name to return the fully-qualified key.
func ResolveConfigKey(name string, namespace ...string) string {
	prefix := strings.Join(namespace, ".") + "."
	if !shouldNamespace(name, prefix) {
		return name
	}
	return prefix + name
}

func shouldNamespace(name, prefix string) bool {
	// already in namespace
	if strings.HasPrefix(name, prefix) {
		return false
	}
	// never namespace lxd limit configuration
	if strings.HasPrefix(name, "limits.") {
		return false
	}
	// never namespace boot config
	if name == "boot.autostart" {
		return false
	}
	return true
}

func splitConfigKey(key string) (namespace, name string) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

// AliveStatuses are the LXD statuses that indicate a container is "alive".
var AliveStatuses = []string{
	// TODO(ericsnow) Also support StatusOK, StatusPending, and StatusThawed?
	StatusStarting,
	StatusStarted,
	StatusRunning,
	StatusStopping,
	StatusStopped,
}

// InstanceSpec holds all the information needed to create a new LXD
// container.
type InstanceSpec struct {
	// Name is the "name" of the instance.
	Name string

	// Image is the name of the image to use.
	Image string

	// ImageRemote identifies the remote to use for images. By default
	// the client's remote is used.
	ImageRemote string

	// Profiles are the names of the container profiles to apply to the
	// new container, in order.
	Profiles []string

	// Ephemeral indicates whether or not the container should be
	// destroyed when the LXD host is restarted.
	Ephemeral bool

	// Metadata is the instance metadata.
	Metadata map[string]string

	// Devices to be added at container initialisation time.
	Devices

	// TODO (mandart 2018-04-25) this has been added here to preserve the
	// method signature in the to-be-deprecated LXD client in tools.
	// The whole InstanceSpec type is redundant as far as the new upstream
	// LXD client is concerned and needs to be removed during a refactor.
	ImageData lxd.SourcedImage

	// TODO(ericsnow) Other possible fields:
	// Disks
	// Networks
	// Metadata
	// Tags
}

func (spec InstanceSpec) config() map[string]string {
	return resolveMetadata(spec.Metadata)
}

func (spec InstanceSpec) info(namespace string) *api.Container {
	name := spec.Name
	if namespace != "" {
		name = namespace + "-" + name
	}

	return &api.Container{
		ContainerPut: api.ContainerPut{
			Architecture: "",
			Config:       spec.config(),
			Devices:      map[string]map[string]string{},
			Ephemeral:    spec.Ephemeral,
			Profiles:     spec.Profiles,
		},
		CreatedAt:       time.Time{},
		ExpandedConfig:  map[string]string{},
		ExpandedDevices: map[string]map[string]string{},
		Name:            name,
		Status:          "",
		StatusCode:      0,
	}
}

// Summary builds an InstanceSummary based on the spec and returns it.
func (spec InstanceSpec) Summary(namespace string) InstanceSummary {
	info := spec.info(namespace)
	return newInstanceSummary(info)
}

// InstanceHardware describes the hardware characteristics of a LXC container.
type InstanceHardware struct {
	// Architecture is the CPU architecture.
	Architecture string

	// NumCores is the number of CPU cores.
	NumCores uint

	// MemoryMB is the memory allocation for the container.
	MemoryMB uint

	// RootDiskMB is the size of the root disk, in MB.
	RootDiskMB uint64
}

// InstanceSummary captures all the data needed by Instance.
type InstanceSummary struct {
	// Name is the "name" of the instance.
	Name string

	// Status holds the status of the instance at a certain point in time.
	Status string

	// Hardware describes the instance's hardware characterstics.
	Hardware InstanceHardware

	// Metadata is the instance metadata.
	Metadata map[string]string

	// Devices is the instance's devices.
	Devices map[string]map[string]string
}

func newInstanceSummary(info *api.Container) InstanceSummary {
	archStr := arch.NormaliseArch(info.Architecture)

	var numCores uint = 0 // default to all
	if raw := info.Config["limits.cpu"]; raw != "" {
		fmt.Sscanf(raw, "%d", &numCores)
	}

	var mem uint = 0 // default to all
	if raw := info.Config["limits.memory"]; raw != "" {
		result, err := shared.ParseByteSizeString(raw)
		if err != nil {
			logger.Errorf("failed to parse %s into bytes, ignoring err: %s", raw, err)
			mem = 0
		} else {
			// We're going to put it into MemoryMB, so adjust by a megabyte
			result = result / megabyte
			if result > math.MaxUint32 {
				logger.Errorf("byte string %s overflowed uint32", raw)
				mem = math.MaxUint32
			} else {
				mem = uint(result)
			}
		}
	}

	// TODO(ericsnow) Factor this out into a function.
	statusStr := info.Status
	for status, code := range allStatuses {
		if info.StatusCode == code {
			statusStr = status
			break
		}
	}

	metadata := extractMetadata(info.Config)

	return InstanceSummary{
		Name:     info.Name,
		Status:   statusStr,
		Metadata: metadata,
		Devices:  info.Devices,
		Hardware: InstanceHardware{
			Architecture: archStr,
			NumCores:     numCores,
			MemoryMB:     mem,
		},
	}
}

// Instance represents a single realized LXD container.
type Instance struct {
	InstanceSummary

	// spec is the InstanceSpec used to create this instance.
	spec *InstanceSpec
}

func newInstance(info *api.Container, spec *InstanceSpec) *Instance {
	summary := newInstanceSummary(info)
	return NewInstance(summary, spec)
}

// NewInstance builds an instance from the provided summary and spec
// and returns it.
func NewInstance(summary InstanceSummary, spec *InstanceSpec) *Instance {
	if spec != nil {
		// Make a copy.
		val := *spec
		spec = &val
	}
	return &Instance{
		InstanceSummary: summary,
		spec:            spec,
	}
}

// Status returns a string identifying the status of the instance.
func (i *Instance) Status() string {
	return i.InstanceSummary.Status
}

// Metadata returns the user-specified metadata for the instance.
func (i *Instance) Metadata() map[string]string {
	// TODO*ericsnow) return a copy?
	return i.InstanceSummary.Metadata
}

// Disks returns the disk devices attached to the instance.
func (i *Instance) Disks() map[string]DiskDevice {
	disks := make(map[string]DiskDevice)
	for name, device := range i.InstanceSummary.Devices {
		if device["type"] != "disk" {
			continue
		}
		disks[name] = DiskDevice{
			Path:     device["path"],
			Source:   device["source"],
			Pool:     device["pool"],
			ReadOnly: device["readonly"] == "true",
		}
	}
	return disks
}

func resolveMetadata(metadata map[string]string) map[string]string {
	config := make(map[string]string)

	for name, val := range metadata {
		key := ResolveConfigKey(name, MetadataNamespace)
		config[key] = val
	}

	return config
}

func extractMetadata(config map[string]string) map[string]string {
	metadata := make(map[string]string)

	for key, val := range config {
		namespace, name := splitConfigKey(key)
		if namespace != MetadataNamespace {
			continue
		}
		metadata[name] = val
	}

	return metadata
}
