// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/ssh"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// FindTags wraps a slice of strings that are prefixes to use when
// searching for matching tags.
type FindTags struct {
	Prefixes []string `json:"prefixes"`
}

// FindTagsResults wraps the mapping between the requested prefix and the
// matching tags for each requested prefix.
type FindTagsResults struct {
	Matches map[string][]Entity `json:"matches"`
}

// Entity identifies a single entity.
type Entity struct {
	Tag string
}

// Entities identifies multiple entities.
type Entities struct {
	Entities []Entity
}

// EntityPasswords holds the parameters for making a SetPasswords call.
type EntityPasswords struct {
	Changes []EntityPassword
}

// EntityPassword specifies a password change for the entity
// with the given tag.
type EntityPassword struct {
	Tag      string
	Password string
}

// ErrorResults holds the results of calling a bulk operation which
// returns no data, only an error result. The order and
// number of elements matches the operations specified in the request.
type ErrorResults struct {
	// Results contains the error results from each operation.
	Results []ErrorResult
}

// OneError returns the error from the result
// of a bulk operation on a single value.
func (result ErrorResults) OneError() error {
	if n := len(result.Results); n != 1 {
		return fmt.Errorf("expected 1 result, got %d", n)
	}
	if err := result.Results[0].Error; err != nil {
		return err
	}
	return nil
}

// Combine returns one error from the result which is an accumulation of the
// errors. If there are no errors in the result, the return value is nil.
// Otherwise the error values are combined with new-line characters.
func (result ErrorResults) Combine() error {
	var errorStrings []string
	for _, r := range result.Results {
		if r.Error != nil {
			errorStrings = append(errorStrings, r.Error.Error())
		}
	}
	if errorStrings != nil {
		return errors.New(strings.Join(errorStrings, "\n"))
	}
	return nil
}

// ErrorResult holds the error status of a single operation.
type ErrorResult struct {
	Error *Error
}

// AddRelation holds the parameters for making the AddRelation call.
// The endpoints specified are unordered.
type AddRelation struct {
	Endpoints []string
}

// AddRelationResults holds the results of a AddRelation call. The Endpoints
// field maps service names to the involved endpoints.
type AddRelationResults struct {
	Endpoints map[string]charm.Relation
}

// DestroyRelation holds the parameters for making the DestroyRelation call.
// The endpoints specified are unordered.
type DestroyRelation struct {
	Endpoints []string
}

// AddCharmWithAuthorization holds the arguments for making an AddCharmWithAuthorization API call.
type AddCharmWithAuthorization struct {
	URL                string
	CharmStoreMacaroon *macaroon.Macaroon
}

// AddMachineParams encapsulates the parameters used to create a new machine.
type AddMachineParams struct {
	// The following fields hold attributes that will be given to the
	// new machine when it is created.
	Series      string                    `json:"Series"`
	Constraints constraints.Value         `json:"Constraints"`
	Jobs        []multiwatcher.MachineJob `json:"Jobs"`

	// Disks describes constraints for disks that must be attached to
	// the machine when it is provisioned.
	Disks []storage.Constraints `json:"Disks"`

	// If Placement is non-nil, it contains a placement directive
	// that will be used to decide how to instantiate the machine.
	Placement *instance.Placement `json:"Placement"`

	// If ParentId is non-empty, it specifies the id of the
	// parent machine within which the new machine will
	// be created. In that case, ContainerType must also be
	// set.
	ParentId string `json:"ParentId"`

	// ContainerType optionally gives the container type of the
	// new machine. If it is non-empty, the new machine
	// will be implemented by a container. If it is specified
	// but ParentId is empty, a new top level machine will
	// be created to hold the container with given series,
	// constraints and jobs.
	ContainerType instance.ContainerType `json:"ContainerType"`

	// If InstanceId is non-empty, it will be associated with
	// the new machine along with the given nonce,
	// hardware characteristics and addresses.
	// All the following fields will be ignored if ContainerType
	// is set.
	InstanceId              instance.Id                      `json:"InstanceId"`
	Nonce                   string                           `json:"Nonce"`
	HardwareCharacteristics instance.HardwareCharacteristics `json:"HardwareCharacteristics"`
	Addrs                   []Address                        `json:"Addrs"`
}

// AddMachines holds the parameters for making the AddMachines call.
type AddMachines struct {
	MachineParams []AddMachineParams `json:"MachineParams"`
}

// AddMachinesResults holds the results of an AddMachines call.
type AddMachinesResults struct {
	Machines []AddMachinesResult `json:"Machines"`
}

// AddMachinesResult holds the name of a machine added by the
// api.client.AddMachine call for a single machine.
type AddMachinesResult struct {
	Machine string `json:"Machine"`
	Error   *Error `json:"Error"`
}

// DestroyMachines holds parameters for the DestroyMachines call.
type DestroyMachines struct {
	MachineNames []string
	Force        bool
}

// ServicesDeploy holds the parameters for deploying one or more services.
type ServicesDeploy struct {
	Services []ServiceDeploy
}

// ServiceDeploy holds the parameters for making the service Deploy call.
type ServiceDeploy struct {
	ServiceName      string
	Series           string
	CharmUrl         string
	NumUnits         int
	Config           map[string]string
	ConfigYAML       string // Takes precedence over config if both are present.
	Constraints      constraints.Value
	Placement        []*instance.Placement
	Networks         []string
	Storage          map[string]storage.Constraints
	EndpointBindings map[string]string
	Resources        map[string]string
}

// ServiceUpdate holds the parameters for making the service Update call.
type ServiceUpdate struct {
	ServiceName     string
	CharmUrl        string
	ForceCharmUrl   bool
	ForceSeries     bool
	MinUnits        *int
	SettingsStrings map[string]string
	SettingsYAML    string // Takes precedence over SettingsStrings if both are present.
	Constraints     *constraints.Value
}

// ServiceSetCharm sets the charm for a given service.
type ServiceSetCharm struct {
	// ServiceName is the name of the service to set the charm on.
	ServiceName string `json:"servicename"`
	// CharmUrl is the new url for the charm.
	CharmUrl string `json:"charmurl"`
	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool `json:"forceunits"`
	// ForceSeries forces the use of the charm even if it doesn't match the
	// series of the unit.
	ForceSeries bool `json:"forceseries"`
	// ResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	ResourceIDs map[string]string `json:"resourceids"`
}

// ServiceExpose holds the parameters for making the service Expose call.
type ServiceExpose struct {
	ServiceName string
}

// ServiceSet holds the parameters for a service Set
// command. Options contains the configuration data.
type ServiceSet struct {
	ServiceName string
	Options     map[string]string
}

// ServiceUnset holds the parameters for a service Unset
// command. Options contains the option attribute names
// to unset.
type ServiceUnset struct {
	ServiceName string
	Options     []string
}

// ServiceGet holds parameters for making the Get or
// GetCharmURL calls.
type ServiceGet struct {
	ServiceName string
}

// ServiceGetResults holds results of the service Get call.
type ServiceGetResults struct {
	Service     string
	Charm       string
	Config      map[string]interface{}
	Constraints constraints.Value
}

// ServiceCharmRelations holds parameters for making the service CharmRelations call.
type ServiceCharmRelations struct {
	ServiceName string
}

// ServiceCharmRelationsResults holds the results of the service CharmRelations call.
type ServiceCharmRelationsResults struct {
	CharmRelations []string
}

// ServiceUnexpose holds parameters for the service Unexpose call.
type ServiceUnexpose struct {
	ServiceName string
}

// ServiceMetricCredential holds parameters for the SetServiceCredentials call.
type ServiceMetricCredential struct {
	ServiceName       string
	MetricCredentials []byte
}

// ServiceMetricCredentials holds multiple ServiceMetricCredential parameters.
type ServiceMetricCredentials struct {
	Creds []ServiceMetricCredential
}

// PublicAddress holds parameters for the PublicAddress call.
type PublicAddress struct {
	Target string
}

// PublicAddressResults holds results of the PublicAddress call.
type PublicAddressResults struct {
	PublicAddress string
}

// PrivateAddress holds parameters for the PrivateAddress call.
type PrivateAddress struct {
	Target string
}

// PrivateAddressResults holds results of the PrivateAddress call.
type PrivateAddressResults struct {
	PrivateAddress string
}

// Resolved holds parameters for the Resolved call.
type Resolved struct {
	UnitName string
	Retry    bool
}

// ResolvedResults holds results of the Resolved call.
type ResolvedResults struct {
	Service  string
	Charm    string
	Settings map[string]interface{}
}

// AddServiceUnitsResults holds the names of the units added by the
// AddUnits call.
type AddServiceUnitsResults struct {
	Units []string
}

// AddServiceUnits holds parameters for the AddUnits call.
type AddServiceUnits struct {
	ServiceName string
	NumUnits    int
	Placement   []*instance.Placement
}

// DestroyServiceUnits holds parameters for the DestroyUnits call.
type DestroyServiceUnits struct {
	UnitNames []string
}

// ServiceDestroy holds the parameters for making the service Destroy call.
type ServiceDestroy struct {
	ServiceName string
}

// Creds holds credentials for identifying an entity.
type Creds struct {
	AuthTag  string
	Password string
	Nonce    string
}

// LoginRequest holds credentials for identifying an entity to the Login v1
// facade. AuthTag holds the tag of the user to connect as. If it is empty,
// then the provided macaroon slices will be used for authentication (if
// any one is valid, the authentication succeeds). If there are no
// valid macaroons and macaroon authentication is configured,
// the LoginResponse will contain a macaroon that when
// discharged, may allow access.
type LoginRequest struct {
	AuthTag     string           `json:"auth-tag"`
	Credentials string           `json:"credentials"`
	Nonce       string           `json:"nonce"`
	Macaroons   []macaroon.Slice `json:"macaroons"`
}

// LoginRequestCompat holds credentials for identifying an entity to the Login v1
// or earlier (v0 or even pre-facade).
type LoginRequestCompat struct {
	LoginRequest
	Creds
}

// GetAnnotationsResults holds annotations associated with an entity.
type GetAnnotationsResults struct {
	Annotations map[string]string
}

// GetAnnotations stores parameters for making the GetAnnotations call.
type GetAnnotations struct {
	Tag string
}

// SetAnnotations stores parameters for making the SetAnnotations call.
type SetAnnotations struct {
	Tag   string
	Pairs map[string]string
}

// GetServiceConstraints stores parameters for making the GetServiceConstraints call.
type GetServiceConstraints struct {
	ServiceName string
}

// GetConstraintsResults holds results of the GetConstraints call.
type GetConstraintsResults struct {
	Constraints constraints.Value
}

// SetConstraints stores parameters for making the SetConstraints call.
type SetConstraints struct {
	ServiceName string //optional, if empty, model constraints are set.
	Constraints constraints.Value
}

// ResolveCharms stores charm references for a ResolveCharms call.
type ResolveCharms struct {
	References []charm.URL
}

// ResolveCharmResult holds the result of resolving a charm reference to a URL, or any error that occurred.
type ResolveCharmResult struct {
	URL   *charm.URL `json:",omitempty"`
	Error string     `json:",omitempty"`
}

// ResolveCharmResults holds results of the ResolveCharms call.
type ResolveCharmResults struct {
	URLs []ResolveCharmResult
}

// AllWatcherId holds the id of an AllWatcher.
type AllWatcherId struct {
	AllWatcherId string
}

// AllWatcherNextResults holds deltas returned from calling AllWatcher.Next().
type AllWatcherNextResults struct {
	Deltas []multiwatcher.Delta
}

// ListSSHKeys stores parameters used for a KeyManager.ListKeys call.
type ListSSHKeys struct {
	Entities
	Mode ssh.ListMode
}

// ModifyUserSSHKeys stores parameters used for a KeyManager.Add|Delete|Import call for a user.
type ModifyUserSSHKeys struct {
	User string
	Keys []string
}

// StateServingInfo holds information needed by a state
// server.
type StateServingInfo struct {
	APIPort   int
	StatePort int
	// The controller cert and corresponding private key.
	Cert       string
	PrivateKey string
	// The private key for the CA cert so that a new controller
	// cert can be generated when needed.
	CAPrivateKey string
	// this will be passed as the KeyFile argument to MongoDB
	SharedSecret   string
	SystemIdentity string
}

// IsMasterResult holds the result of an IsMaster API call.
type IsMasterResult struct {
	// Master reports whether the connected agent
	// lives on the same instance as the mongo replica
	// set master.
	Master bool
}

// ContainerManagerConfigParams contains the parameters for the
// ContainerManagerConfig provisioner API call.
type ContainerManagerConfigParams struct {
	Type instance.ContainerType
}

// ContainerManagerConfig contains information from the model config
// that is needed for configuring the container manager.
type ContainerManagerConfig struct {
	ManagerConfig map[string]string
}

// UpdateBehavior contains settings that are duplicated in several
// places. Let's just embed this instead.
type UpdateBehavior struct {
	EnableOSRefreshUpdate bool
	EnableOSUpgrade       bool
}

// ContainerConfig contains information from the model config that is
// needed for container cloud-init.
type ContainerConfig struct {
	ProviderType            string
	AuthorizedKeys          string
	SSLHostnameVerification bool
	Proxy                   proxy.Settings
	AptProxy                proxy.Settings
	AptMirror               string
	PreferIPv6              bool
	AllowLXCLoopMounts      bool
	*UpdateBehavior
}

// ProvisioningScriptParams contains the parameters for the
// ProvisioningScript client API call.
type ProvisioningScriptParams struct {
	MachineId string
	Nonce     string

	// DataDir may be "", in which case the default will be used.
	DataDir string

	// DisablePackageCommands may be set to disable all
	// package-related commands. It is then the responsibility of the
	// provisioner to ensure that all the packages required by Juju
	// are available.
	DisablePackageCommands bool
}

// ProvisioningScriptResult contains the result of the
// ProvisioningScript client API call.
type ProvisioningScriptResult struct {
	Script string
}

// DeployerConnectionValues containers the result of deployer.ConnectionInfo
// API call.
type DeployerConnectionValues struct {
	StateAddresses []string
	APIAddresses   []string
}

// JobsResult holds the jobs for a machine that are returned by a call to Jobs.
type JobsResult struct {
	Jobs  []multiwatcher.MachineJob `json:"Jobs"`
	Error *Error                    `json:"Error"`
}

// JobsResults holds the result of a call to Jobs.
type JobsResults struct {
	Results []JobsResult `json:"Results"`
}

// DistributionGroupResult contains the result of
// the DistributionGroup provisioner API call.
type DistributionGroupResult struct {
	Error  *Error
	Result []instance.Id
}

// DistributionGroupResults is the bulk form of
// DistributionGroupResult.
type DistributionGroupResults struct {
	Results []DistributionGroupResult
}

// FacadeVersions describes the available Facades and what versions of each one
// are available
type FacadeVersions struct {
	Name     string
	Versions []int
}

// LoginResult holds the result of a Login call.
type LoginResult struct {
	Servers        [][]HostPort     `json:"Servers"`
	ModelTag       string           `json:"ModelTag"`
	LastConnection *time.Time       `json:"LastConnection"`
	Facades        []FacadeVersions `json:"Facades"`
}

// ReauthRequest holds a challenge/response token meaningful to the identity
// provider.
type ReauthRequest struct {
	Prompt string `json:"prompt"`
	Nonce  string `json:"nonce"`
}

// AuthUserInfo describes a logged-in local user or remote identity.
type AuthUserInfo struct {
	DisplayName    string     `json:"display-name"`
	Identity       string     `json:"identity"`
	LastConnection *time.Time `json:"last-connection,omitempty"`

	// Credentials contains an optional opaque credential value to be held by
	// the client, if any.
	Credentials *string `json:"credentials,omitempty"`
}

// LoginResultV1 holds the result of an Admin v1 Login call.
type LoginResultV1 struct {
	// DischargeRequired implies that the login request has failed, and none of
	// the other fields are populated. It contains a macaroon which, when
	// discharged, will grant access on a subsequent call to Login.
	// Note: It is OK to use the Macaroon type here as it is explicitely
	// designed to provide stable serialisation of macaroons.  It's good
	// practice to only use primitives in types that will be serialised,
	// however because of the above it is suitable to use the Macaroon type
	// here.
	DischargeRequired *macaroon.Macaroon `json:"discharge-required,omitempty"`

	// DischargeRequiredReason holds the reason that the above discharge was
	// required.
	DischargeRequiredReason string `json:"discharge-required-error,omitempty"`

	// Servers is the list of API server addresses.
	Servers [][]HostPort `json:"servers,omitempty"`

	// ModelTag is the tag for the model that is being connected to.
	ModelTag string `json:"model-tag,omitempty"`

	// ControllerTag is the tag for the model that holds the API servers.
	// This is the initial model created when bootstrapping juju.
	ControllerTag string `json:"server-tag,omitempty"`

	// UserInfo describes the authenticated user, if any.
	UserInfo *AuthUserInfo `json:"user-info,omitempty"`

	// Facades describes all the available API facade versions to the
	// authenticated client.
	Facades []FacadeVersions `json:"facades,omitempty"`

	// ServerVersion is the string representation of the server version
	// if the server supports it.
	ServerVersion string `json:"server-version,omitempty"`
}

// ControllersServersSpec contains arguments for
// the EnableHA client API call.
type ControllersSpec struct {
	ModelTag       string
	NumControllers int               `json:"num-controllers"`
	Constraints    constraints.Value `json:"constraints,omitempty"`
	// Series is the series to associate with new controller machines.
	// If this is empty, then the model's default series is used.
	Series string `json:"series,omitempty"`
	// Placement defines specific machines to become new controller machines.
	Placement []string `json:"placement,omitempty"`
}

// ControllersServersSpecs contains all the arguments
// for the EnableHA API call.
type ControllersSpecs struct {
	Specs []ControllersSpec
}

// ControllersChangeResult contains the results
// of a single EnableHA API call or
// an error.
type ControllersChangeResult struct {
	Result ControllersChanges
	Error  *Error
}

// ControllersChangeResults contains the results
// of the EnableHA API call.
type ControllersChangeResults struct {
	Results []ControllersChangeResult
}

// ControllersChanges lists the servers
// that have been added, removed or maintained in the
// pool as a result of an enable-ha operation.
type ControllersChanges struct {
	Added      []string `json:"added,omitempty"`
	Maintained []string `json:"maintained,omitempty"`
	Removed    []string `json:"removed,omitempty"`
	Promoted   []string `json:"promoted,omitempty"`
	Demoted    []string `json:"demoted,omitempty"`
	Converted  []string `json:"converted,omitempty"`
}

// FindToolsParams defines parameters for the FindTools method.
type FindToolsParams struct {
	// Number will be used to match tools versions exactly if non-zero.
	Number version.Number

	// MajorVersion will be used to match the major version if non-zero.
	MajorVersion int

	// MinorVersion will be used to match the major version if greater
	// than or equal to zero, and Number is zero.
	MinorVersion int

	// Arch will be used to match tools by architecture if non-empty.
	Arch string

	// Series will be used to match tools by series if non-empty.
	Series string
}

// FindToolsResult holds a list of tools from FindTools and any error.
type FindToolsResult struct {
	List  tools.List
	Error *Error
}

// ImageFilterParams holds the parameters used to specify images to delete.
type ImageFilterParams struct {
	Images []ImageSpec `json:"images"`
}

// ImageSpec defines the parameters to select images list or delete.
type ImageSpec struct {
	Kind   string `json:"kind"`
	Arch   string `json:"arch"`
	Series string `json:"series"`
}

// ListImageResult holds the results of querying images.
type ListImageResult struct {
	Result []ImageMetadata `json:"result"`
}

// ImageMetadata represents an image in storage.
type ImageMetadata struct {
	Kind    string    `json:"kind"`
	Arch    string    `json:"arch"`
	Series  string    `json:"series"`
	URL     string    `json:"url"`
	Created time.Time `json:"created"`
}

// RebootActionResults holds a list of RebootActionResult and any error.
type RebootActionResults struct {
	Results []RebootActionResult `json:"results,omitempty"`
}

// RebootActionResult holds the result of a single call to
// machine.ShouldRebootOrShutdown.
type RebootActionResult struct {
	Result RebootAction `json:"result,omitempty"`
	Error  *Error       `json:"error,omitempty"`
}

// LogRecord is used to transmit log messages to the logsink API
// endpoint.  Single character field names are used for serialisation
// to keep the size down. These messages are going to be sent a lot.
type LogRecord struct {
	Time     time.Time   `json:"t"`
	Module   string      `json:"m"`
	Location string      `json:"l"`
	Level    loggo.Level `json:"v"`
	Message  string      `json:"x"`
}

// GetBundleChangesParams holds parameters for making GetBundleChanges calls.
type GetBundleChangesParams struct {
	// BundleDataYAML is the YAML-encoded charm bundle data
	// (see "github.com/juju/charm.BundleData").
	BundleDataYAML string `json:"yaml"`
}

// GetBundleChangesResults holds results of the GetBundleChanges call.
type GetBundleChangesResults struct {
	// Changes holds the list of changes required to deploy the bundle.
	// It is omitted if the provided bundle YAML has verification errors.
	Changes []*BundleChangesChange `json:"changes,omitempty"`
	// Errors holds possible bundle verification errors.
	Errors []string `json:"errors,omitempty"`
}

// BundleChangesChange holds a single change required to deploy a bundle.
type BundleChangesChange struct {
	// Id is the unique identifier for this change.
	Id string `json:"id"`
	// Method is the action to be performed to apply this change.
	Method string `json:"method"`
	// Args holds a list of arguments to pass to the method.
	Args []interface{} `json:"args"`
	// Requires holds a list of dependencies for this change. Each dependency
	// is represented by the corresponding change id, and must be applied
	// before this change is applied.
	Requires []string `json:"requires"`
}

// UpgradeMongoParams holds the arguments required to
// enter upgrade mongo mode.
type UpgradeMongoParams struct {
	Target mongo.Version
}

// HAMember holds information that identifies one member
// of HA.
type HAMember struct {
	Tag           string
	PublicAddress network.Address
	Series        string
}

// MongoUpgradeResults holds the results of an attempt
// to enter upgrade mongo mode.
type MongoUpgradeResults struct {
	RsMembers []replicaset.Member
	Master    HAMember
	Members   []HAMember
}

// ResumeReplicationParams holds the members of a HA that
// must be resumed.
type ResumeReplicationParams struct {
	Members []replicaset.Member
}

// ModelInfo holds information about the Juju model.
type ModelInfo struct {
	DefaultSeries string `json:"DefaultSeries"`
	ProviderType  string `json:"ProviderType"`
	Name          string `json:"Name"`
	UUID          string `json:"UUID"`
	// The json name here is as per the older field name and is required
	// for backward compatability. The other fields also have explicit
	// matching serialization directives for the benefit of being explicit.
	ControllerUUID string `json:"ServerUUID"`
}

// MeterStatusParam holds meter status information to be set for the specified tag.
type MeterStatusParam struct {
	Tag  string `json:"tag"`
	Code string `json:"code"`
	Info string `json:"info, omitempty"`
}

// MeterStatusParams holds parameters for making SetMeterStatus calls.
type MeterStatusParams struct {
	Statuses []MeterStatusParam `json:"statues"`
}
