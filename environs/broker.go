// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/tools"
)

// StatusCallbackFunc represents a function that can be called to report a status.
type StatusCallbackFunc func(ctx context.Context, settableStatus status.Status, info string, data map[string]interface{}) error

// StartInstanceParams holds parameters for the
// InstanceBroker.StartInstance method.
type StartInstanceParams struct {
	// ControllerUUID is the uuid of the controller.
	ControllerUUID string

	// Constraints is a set of constraints on
	// the kind of instance to create.
	Constraints constraints.Value

	// Tools is a list of tools that may be used
	// to start a Juju agent on the machine.
	Tools tools.List

	// InstanceConfig describes the machine's configuration.
	InstanceConfig *instancecfg.InstanceConfig

	// Placement, if non-empty, contains an environment-specific
	// placement directive that may be used to decide how the
	// instance should be started.
	Placement string

	// AvailabilityZone, provides the name of the availability
	// zone required to start the instance.
	AvailabilityZone string

	// RootDisk is a set of parameters for creating the root disk volume.
	RootDisk *storage.VolumeParams

	// Volumes is a set of parameters for volumes that should be created.
	//
	// StartInstance need not check the value of the Attachment field,
	// as it is guaranteed that any volumes in this list are designated
	// for attachment to the instance being started.
	Volumes []storage.VolumeParams

	// VolumeAttachments is a set of parameters for existing volumes that
	// should be attached. If the StartInstance method does not attach the
	// volumes, they will be attached by the storage provisioner once the
	// machine has been created. The attachments are presented here to
	// give the provider an opportunity for the volume attachments to
	// influence the instance creation, e.g. by restricting the machine
	// to specific availability zones.
	VolumeAttachments []storage.VolumeAttachmentParams

	// NetworkInfo is an optional list of network interface details,
	// necessary to configure on the instance.
	NetworkInfo corenetwork.InterfaceInfos

	// SubnetsToZones is an optional collection of maps of provider-specific
	// subnet IDs to a list of availability zone names each subnet is available
	// in.
	// It is populated when valid positive spaces constraints are present,
	// or when the instance to be started will host an application with bound
	// endpoints.
	// The subnets present are derived from the union of
	// constrained/bound spaces.
	// Negative space constraints will have already been validated when
	// retrieving `ProvisioningInfo`.
	SubnetsToZones []map[corenetwork.Id][]string

	// EndpointBindings holds the mapping between application endpoint names to
	// provider-specific space IDs. It is populated when provisioning a machine
	// to host a unit of an application with endpoint bindings.
	EndpointBindings map[string]corenetwork.Id

	// ImageMetadata is a collection of image metadata
	// that may be used to start this instance.
	ImageMetadata []*imagemetadata.ImageMetadata

	// CleanupCallback is a callback to be used to clean up any residual
	// status-reporting output from StatusCallback.
	CleanupCallback func() error

	// StatusCallback is a callback to be used by the instance to report
	// changes in status. Its signature is consistent with other
	// status-related functions to allow them to be used as callbacks.
	StatusCallback StatusCallbackFunc

	// Abort is a channel that will be closed to indicate that the command
	// should be aborted.
	Abort <-chan struct{}

	// CharmLXDProfiles is a slice of names of lxd profiles to be used creating
	// the LXD container, if specified and an LXD container.  The profiles
	// come from charms deployed on the machine.
	CharmLXDProfiles []string
}

// StartInstanceResult holds the result of an
// InstanceBroker.StartInstance method call.
type StartInstanceResult struct {
	// DisplayName is an optional human-readable string that's used
	// for display purposes only.
	DisplayName string

	// Instance is an interface representing a cloud instance.
	Instance instances.Instance

	// Config holds the environment config to be used for any further
	// operations, if the instance is for a controller.
	Config *config.Config

	// HardwareCharacteristics represents the hardware characteristics
	// of the newly created instance.
	Hardware *instance.HardwareCharacteristics

	// NetworkInfo contains information about how to configure network
	// interfaces on the instance. Depending on the provider, this
	// might be the same StartInstanceParams.NetworkInfo or may be
	// modified as needed.
	NetworkInfo corenetwork.InterfaceInfos

	// Volumes contains a list of volumes created, each one having the
	// same Name as one of the VolumeParams in StartInstanceParams.Volumes.
	// VolumeAttachment information is reported separately.
	Volumes []storage.Volume

	// VolumeAttachments contains a attachment-specific information about
	// volumes that were attached to the started instance.
	VolumeAttachments []storage.VolumeAttachment
}

// InstanceBroker defines methods for managing vm or container instances.
// TODO(wallyworld) - we want this in the environs/instance package but import loops
// stop that from being possible right now.
type InstanceBroker interface {
	// StartInstance asks for a new instance to be created, associated with
	// the provided config in machineConfig. The given config describes the
	// juju state for the new instance to connect to. The config
	// MachineNonce, which must be unique within an environment, is used by
	// juju to protect against the consequences of multiple instances being
	// started with the same machine id.
	//
	// Callers may attempt to distribute instances across a set of
	// availability zones. If one zone fails, then the caller is expected
	// to attempt in another zone. If the provider can determine that
	// the StartInstanceParams can never be fulfilled in any zone, then
	// it may return an error satisfying Is(err, ErrAvailabilityZoneIndependent).
	StartInstance(ctx context.Context, args StartInstanceParams) (*StartInstanceResult, error)

	// StopInstances shuts down the instances with the specified IDs.
	// Unknown instance IDs are ignored, to enable idempotency.
	StopInstances(context.Context, ...instance.Id) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances(ctx context.Context) ([]instances.Instance, error)

	// AllRunningInstances returns all running, available instances currently known to the broker.
	AllRunningInstances(ctx context.Context) ([]instances.Instance, error)
}

// LXDProfiler defines an interface for dealing with lxd profiles used to
// deploy juju machines and containers.
type LXDProfiler interface {
	// SupportsLXDProfiles indicates whether this environ supports
	// interacting with LXD Profiles.
	SupportsLXDProfiles() bool
	// AssignLXDProfiles assigns the given profile names to the lxd instance
	// provided.  The slice of ProfilePosts provides details for adding to
	// and removing profiles from the lxd server.
	AssignLXDProfiles(instID string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) ([]string, error)

	// MaybeWriteLXDProfile writes the given LXDProfile to if not already there.
	MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error

	// LXDProfileNames returns all the profiles associated to a container name
	LXDProfileNames(containerName string) ([]string, error)
}

// NoLXDProfiler implements methods from LXDProfiler that represent
// an environ without the ability interact with LXD Profiles.
// As with NoSpaceDiscoveryEnviron it can be embedded safely.
type NoLXDProfiler struct{}

func (*NoLXDProfiler) SupportsLXDProfiles() bool {
	return false
}

func (*NoLXDProfiler) AssignLXDProfiles(string, []string, []lxdprofile.ProfilePost) ([]string, error) {
	return nil, errors.NotSupportedf("AssignLXDProfiles")
}

func (*NoLXDProfiler) MaybeWriteLXDProfile(string, lxdprofile.Profile) error {
	return errors.NotSupportedf("AssignLXDProfiles")
}

func (*NoLXDProfiler) LXDProfileNames(string) ([]string, error) {
	return nil, errors.NotSupportedf("AssignLXDProfiles")
}
