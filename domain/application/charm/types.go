// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/version/v2"

	corecharm "github.com/juju/juju/core/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// GetCharmArgs holds the arguments for the GetCharmID method.
// Name is the name of the charm to get the ID for. This is required.
// Revision allows the selection of a specific revision of the charm. If
// revision is not provided, the latest revision is returned.
type GetCharmArgs struct {
	// Name is the name of the charm to get the ID for.
	Name string

	// Revision allows the selection of a specific revision of the charm.
	// Otherwise, the latest revision is returned.
	Revision *int

	// Source is the source of the charm.
	Source CharmSource
}

// CharmSource represents the source of a charm.
type CharmSource string

const (
	// LocalSource represents a local charm source.
	LocalSource CharmSource = "local"
	// CharmHubSource represents a charmhub charm source.
	CharmHubSource CharmSource = "charmhub"
)

// ParseCharmSchema creates a CharmSource from a  string.
// It will map the string "ch" (representing the CharmHub URL scheme) to
// CharmHubSource and "local" to LocalSource.
func ParseCharmSchema(source internalcharm.Schema) (CharmSource, error) {
	switch source {
	case internalcharm.Local:
		return LocalSource, nil
	case internalcharm.CharmHub:
		return CharmHubSource, nil
	default:
		return "", errors.Errorf("%w: %v", applicationerrors.CharmSourceNotValid, source)
	}
}

// SetCharmArgs holds the arguments for the SetCharm method.
type SetCharmArgs struct {
	// Charm is the charm to set.
	Charm internalcharm.Charm
	// Source is the source of the charm.
	Source internalcharm.Schema
	// ReferenceName is the given name of the charm that is stored in the
	// persistent storage. The proxy name should either be the application name
	// or the charm metadata name.
	//
	// The name of a charm can differ from the charm name stored in the metadata
	// in the cases where the application name is selected by the user. In order
	// to select that charm again via the name, we need to use the proxy name to
	// locate it. You can't go via the application and select it via the
	// application name, as no application might be referencing it at that
	// specific revision. The only way to then locate the charm directly via the
	// name is use the proxy name.
	ReferenceName string
	// Revision is the revision of the charm.
	Revision int
	// Hash is the hash of the charm.
	Hash string
	// ArchivePath is the path to the charm archive path.
	ArchivePath string
	// Version is the optional charm version.
	Version string
}

// Revision is the charm revision.
// This type alias just exists to help understand what an int represents in
// the context of return arguments.
type Revision = int

// SetStateArgs holds the arguments for the SetState method.
type SetStateArgs struct {
	// Source is the source of the charm.
	Source CharmSource
	// ReferenceName is the given name of the charm that is stored in the
	// persistent storage. The proxy name should either be the application name
	// or the charm metadata name.
	//
	// The name of a charm can differ from the charm name stored in the metadata
	// in the cases where the application name is selected by the user. In order
	// to select that charm again via the name, we need to use the proxy name to
	// locate it. You can't go via the application and select it via the
	// application name, as no application might be referencing it at that
	// specific revision. The only way to then locate the charm directly via the
	// name is use the proxy name.
	ReferenceName string
	// Revision is the revision of the charm.
	Revision Revision
	// Hash is the hash of the charm.
	Hash string
	// ArchivePath is the path to the charm archive path.
	ArchivePath string
	// Version is the optional charm version.
	Version string
	// Platform is the platform of the charm.
	Platform Platform
}

type CharmInfo struct {
	// UUID is the UUID of the charm.
	UUID corecharm.ID
	// Name is the name of the charm.
	Name string
	// Origin holds additional origin information about the charm.
	Origin CharmOrigin
}

// Charm represents a charm from the perspective of the service. This is the
// golden source of charm information. If the charm changes at the wire format
// level, we should be able to map it to this struct.
type Charm struct {
	// Metadata holds the metadata of the charm.
	Metadata Metadata
	// Manifest holds the manifest of the charm. It defines the bases that
	// the charm supports.
	Manifest Manifest
	// Actions holds the actions of the charm.
	Actions Actions
	// Config holds the configuration options of the charm.
	Config Config
	// LXDProfile holds the LXD profile of the charm. It allows the charm to
	// specify the LXD profile that should be used when deploying the charm.
	LXDProfile []byte
}

// CharmOrigin holds additional origin about a charm, that is not part of
// the charm metadata or manifest. The origin holds information for a charm
// regarding where it came from.
type CharmOrigin struct {
	// ReferenceName is the given name of the charm that is stored in the
	// persistent storage. The proxy name should either be the application name
	// or the charm metadata name.
	//
	// The name of a charm can differ from the charm name stored in the metadata
	// in the cases where the application name is selected by the user. In order
	// to select that charm again via the name, we need to use the proxy name to
	// locate it. You can't go via the application and select it via the
	// application name, as no application might be referencing it at that
	// specific revision. The only way to then locate the charm directly via the
	// name is use the proxy name.
	ReferenceName string
	// Source is the source of the charm. This is either local or charmhub.
	Source CharmSource
	// Revision is the revision of the charm.
	Revision int
	// Platform is the platform of the charm.
	Platform Platform
}

// CharmWithOrigin represents a charm with its origin.
type CharmWithOrigin struct {
	CharmOrigin

	// Name is the name of the charm.
	Name string
}

// OSType represents the type of an application's OS.
type OSType int

const (
	Ubuntu OSType = iota
)

// Architecture represents an application's architecture.
type Architecture int

const (
	AMD64 Architecture = iota
	ARM64
	PPC64EL
	S390X
	RISV64
)

// Platform contains parameters for an application's platform.
type Platform struct {
	Channel      string
	OSType       OSType
	Architecture Architecture
}

// Metadata represents the metadata of a charm from the perspective of the
// service. This is the golden source of charm metadata. If the charm changes
// at the wire format level, we should be able to map it to this struct.
//
// Of note:
//   - Assumes is represented as a single binary blob and not of hierarchical
//     set of structs.
//   - RunAs default value is marshalled as "default" and not as an empty
//     string.
type Metadata struct {
	Name           string
	Summary        string
	Description    string
	Subordinate    bool
	Provides       map[string]Relation
	Requires       map[string]Relation
	Peers          map[string]Relation
	ExtraBindings  map[string]ExtraBinding
	Categories     []string
	Tags           []string
	Storage        map[string]Storage
	Devices        map[string]Device
	PayloadClasses map[string]PayloadClass
	Resources      map[string]Resource
	Terms          []string
	MinJujuVersion version.Number
	Containers     map[string]Container
	Assumes        []byte
	RunAs          RunAs
}

// RunAs defines which user to run a certain process as.
type RunAs string

const (
	RunAsDefault RunAs = "default"
	RunAsRoot    RunAs = "root"
	RunAsSudoer  RunAs = "sudoer"
	RunAsNonRoot RunAs = "non-root"
)

// RelationRole defines the role of a relation.
type RelationRole string

const (
	RoleProvider RelationRole = "provider"
	RoleRequirer RelationRole = "requirer"
	RolePeer     RelationRole = "peer"
)

// RelationScope describes the scope of a relation.
type RelationScope string

// Note that schema doesn't support custom string types,
// so when we use these values in a schema.Checker,
// we must store them as strings, not RelationScopes.

const (
	ScopeGlobal    RelationScope = "global"
	ScopeContainer RelationScope = "container"
)

// Relation represents a single relation defined in the charm
// metadata.yaml file.
type Relation struct {
	Key       string
	Name      string
	Role      RelationRole
	Interface string
	Optional  bool
	Limit     int
	Scope     RelationScope
}

// ExtraBinding represents an extra bindable endpoint that is not a relation.
type ExtraBinding struct {
	Name string
}

// StorageType defines a storage type.
type StorageType string

const (
	StorageBlock      StorageType = "block"
	StorageFilesystem StorageType = "filesystem"
)

// Storage represents a charm's storage requirement.
type Storage struct {
	// Name is the name of the store.
	//
	// Name has no default, and must be specified.
	Name string

	// Description is a description of the store.
	//
	// Description has no default, and is optional.
	Description string

	// Type is the storage type: filesystem or block-device.
	//
	// Type has no default, and must be specified.
	Type StorageType

	// Shared indicates that the storage is shared between all units of
	// an application deployed from the charm. It is an error to attempt to
	// assign non-shareable storage to a "shared" storage requirement.
	//
	// Shared defaults to false.
	Shared bool

	// ReadOnly indicates that the storage should be made read-only if
	// possible. If the storage cannot be made read-only, Juju will warn
	// the user.
	//
	// ReadOnly defaults to false.
	ReadOnly bool

	// CountMin is the number of storage instances that must be attached
	// to the charm for it to be useful; the charm will not install until
	// this number has been satisfied. This must be a non-negative number.
	//
	// CountMin defaults to 1 for singleton stores.
	CountMin int

	// CountMax is the largest number of storage instances that can be
	// attached to the charm. If CountMax is -1, then there is no upper
	// bound.
	//
	// CountMax defaults to 1 for singleton stores.
	CountMax int

	// MinimumSize is the minimum size of store that the charm needs to
	// work at all. This is not a recommended size or a comfortable size
	// or a will-work-well size, just a bare minimum below which the charm
	// is going to break.
	// MinimumSize requires a unit, one of MGTPEZY, and is stored as MiB.
	//
	// There is no default MinimumSize; if left unspecified, a provider
	// specific default will be used, typically 1GB for block storage.
	MinimumSize uint64

	// Location is the mount location for filesystem stores. For multi-
	// stores, the location acts as the parent directory for each mounted
	// store.
	//
	// Location has no default, and is optional.
	Location string

	// Properties allow the charm author to characterise the relative storage
	// performance requirements and sensitivities for each store.
	// eg “transient” is used to indicate that non persistent storage is
	// acceptable, such as tmpfs or ephemeral instance disks.
	//
	// Properties has no default, and is optional.
	Properties []string
}

// DeviceType defines a device type.
type DeviceType string

// Device represents a charm's device requirement (GPU for example).
type Device struct {
	// Name is the name of the device.
	Name string

	// Description is a description of the device.
	Description string

	// Type is the device type.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType

	// CountMin is the min number of devices that the charm requires.
	CountMin int64

	// CountMax is the max number of devices that the charm requires.
	CountMax int64
}

// PayloadClass holds the information about a payload class, as stored
// in a charm's metadata.
type PayloadClass struct {
	// Name identifies the payload class.
	Name string

	// Type identifies the type of payload (e.g. kvm, docker).
	Type string
}

// Type enumerates the recognized resource types.
type ResourceType string

const (
	ResourceTypeFile           ResourceType = "file"
	ResourceTypeContainerImage ResourceType = "oci-image"
)

// Resource holds the information about a resource, as stored
// in a charm's metadata.
type Resource struct {
	// Name identifies the resource.
	Name string

	// Type identifies the type of resource (e.g. "file").
	Type ResourceType

	// Path is the relative path of the file or directory where the
	// resource will be stored under the unit's data directory. The path
	// is resolved against a subdirectory assigned to the resource. For
	// example, given an application named "spam", a resource "eggs", and a
	// path "eggs.tgz", the fully resolved storage path for the resource
	// would be:
	//   /var/lib/juju/agent/spam-0/resources/eggs/eggs.tgz
	Path string

	// Description holds optional user-facing info for the resource.
	Description string
}

// Container specifies the possible systems it supports and mounts it wants.
type Container struct {
	Resource string
	Mounts   []Mount
	Uid      *int
	Gid      *int
}

// Mount allows a container to mount a storage filesystem from the storage
// top-level directive.
type Mount struct {
	Storage  string
	Location string
}

// Manifest represents the manifest of a charm from the perspective of the
// service. This is the golden source of charm manifest. If the charm changes
// at the wire format level, we should be able to map it to this struct.
type Manifest struct {
	Bases []Base
}

// Base represents an OS/Channel and architecture combination that a charm
// supports.
type Base struct {
	Name          string
	Channel       Channel
	Architectures []string
}

// Channel represents the channel of a charm.
type Channel struct {
	Track  string
	Risk   ChannelRisk
	Branch string
}

// ChannelRisk describes the type of risk in a current channel.
type ChannelRisk string

const (
	RiskStable    ChannelRisk = "stable"
	RiskCandidate ChannelRisk = "candidate"
	RiskBeta      ChannelRisk = "beta"
	RiskEdge      ChannelRisk = "edge"
)

// Actions defines the available actions for the charm. Additional params
// may be added as metadata at a future time (e.g. version.)
type Actions struct {
	Actions map[string]Action
}

// Action is a definition of the parameters and traits of an Action.
type Action struct {
	Description    string
	Parallel       bool
	ExecutionGroup string
	Params         []byte
}

// Config represents the supported configuration options for a charm,
// as declared in its config.yaml file.
type Config struct {
	Options map[string]Option
}

// OptionType defines the type of a charm config option.
type OptionType string

const (
	OptionString OptionType = "string"
	OptionInt    OptionType = "int"
	OptionFloat  OptionType = "float"
	OptionBool   OptionType = "boolean"
	OptionSecret OptionType = "secret"
)

// Option represents a single charm config option.
type Option struct {
	Type        OptionType
	Description string
	Default     any
}
