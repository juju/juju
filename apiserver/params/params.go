// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/utils/ssh"
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

// AddMachines holds the parameters for making the
// AddMachinesWithPlacement call.
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

// ServiceDeploy holds the parameters for making the ServiceDeploy call.
type ServiceDeploy struct {
	ServiceName   string
	CharmUrl      string
	NumUnits      int
	Config        map[string]string
	ConfigYAML    string // Takes precedence over config if both are present.
	Constraints   constraints.Value
	ToMachineSpec string
	Placement     []*instance.Placement
	Networks      []string
	Storage       map[string]storage.Constraints
}

// ServiceUpdate holds the parameters for making the ServiceUpdate call.
type ServiceUpdate struct {
	ServiceName     string
	CharmUrl        string
	ForceCharmUrl   bool
	MinUnits        *int
	SettingsStrings map[string]string
	SettingsYAML    string // Takes precedence over SettingsStrings if both are present.
	Constraints     *constraints.Value
}

// ServiceSetCharm sets the charm for a given service.
type ServiceSetCharm struct {
	ServiceName string
	CharmUrl    string
	Force       bool
}

// ServiceExpose holds the parameters for making the ServiceExpose call.
type ServiceExpose struct {
	ServiceName string
}

// ServiceSet holds the parameters for a ServiceSet
// command. Options contains the configuration data.
type ServiceSet struct {
	ServiceName string
	Options     map[string]string
}

// ServiceSetYAML holds the parameters for
// a ServiceSetYAML command. Config contains the
// configuration data in YAML format.
type ServiceSetYAML struct {
	ServiceName string
	Config      string
}

// ServiceUnset holds the parameters for a ServiceUnset
// command. Options contains the option attribute names
// to unset.
type ServiceUnset struct {
	ServiceName string
	Options     []string
}

// ServiceGet holds parameters for making the ServiceGet or
// ServiceGetCharmURL calls.
type ServiceGet struct {
	ServiceName string
}

// ServiceGetResults holds results of the ServiceGet call.
type ServiceGetResults struct {
	Service     string
	Charm       string
	Config      map[string]interface{}
	Constraints constraints.Value
}

// ServiceCharmRelations holds parameters for making the ServiceCharmRelations call.
type ServiceCharmRelations struct {
	ServiceName string
}

// ServiceCharmRelationsResults holds the results of the ServiceCharmRelations call.
type ServiceCharmRelationsResults struct {
	CharmRelations []string
}

// ServiceUnexpose holds parameters for the ServiceUnexpose call.
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
// AddServiceUnits call.
type AddServiceUnitsResults struct {
	Units []string
}

// AddServiceUnits holds parameters for the AddUnits call.
type AddServiceUnits struct {
	ServiceName   string
	NumUnits      int
	ToMachineSpec string
	Placement     []*instance.Placement
}

// DestroyServiceUnits holds parameters for the DestroyUnits call.
type DestroyServiceUnits struct {
	UnitNames []string
}

// ServiceDestroy holds the parameters for making the ServiceDestroy call.
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
// facade.
type LoginRequest struct {
	AuthTag     string `json:"auth-tag"`
	Credentials string `json:"credentials"`
	Nonce       string `json:"nonce"`
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
	ServiceName string //optional, if empty, environment constraints are set.
	Constraints constraints.Value
}

// ResolveCharms stores charm references for a ResolveCharms call.
type ResolveCharms struct {
	References []charm.Reference
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
	// The state server cert and corresponding private key.
	Cert       string
	PrivateKey string
	// The private key for the CA cert so that a new state server
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

// ContainerManagerConfig contains information from the environment config
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

// ContainerConfig contains information from the environment config that is
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

// SetRsyslogCertParams holds parameters for the SetRsyslogCert call.
type SetRsyslogCertParams struct {
	CACert []byte
	CAKey  []byte
}

// RsyslogConfigResult holds the result of a GetRsyslogConfig call.
type RsyslogConfigResult struct {
	Error  *Error `json:"Error"`
	CACert string `json:"CACert"`
	CAKey  string `json:"CAKey"`
	// Port is only used by state servers as the port to listen on.
	// Clients should use HostPorts for the rsyslog addresses to forward
	// logs to.
	Port int `json:"Port"`

	HostPorts []HostPort `json:"HostPorts"`
}

// RsyslogConfigResults is the bulk form of RyslogConfigResult
type RsyslogConfigResults struct {
	Results []RsyslogConfigResult
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
	EnvironTag     string           `json:"EnvironTag"`
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
	// Servers is the list of API server addresses.
	Servers [][]HostPort `json:"servers"`

	// EnvironTag is the tag for the environment that is being connected to.
	EnvironTag string `json:"environ-tag"`

	// ServerTag is the tag for the environment that holds the API servers.
	// This is the initial environment created when bootstrapping juju.
	ServerTag string `json:"server-tag"`

	// ReauthRequest can be used to relay any further authentication handshaking
	// required on the part of the client to complete the Login, if any.
	ReauthRequest *ReauthRequest `json:"reauth-request,omitempty"`

	// UserInfo describes the authenticated user, if any.
	UserInfo *AuthUserInfo `json:"user-info,omitempty"`

	// Facades describes all the available API facade versions to the
	// authenticated client.
	Facades []FacadeVersions `json:"facades"`

	// ServerVersion is the string representation of the server version
	// if the server supports it.
	ServerVersion string `json:"server-version,omitempty"`
}

// StateServersSpec contains arguments for
// the EnsureAvailability client API call.
type StateServersSpec struct {
	EnvironTag      string
	NumStateServers int               `json:"num-state-servers"`
	Constraints     constraints.Value `json:"constraints,omitempty"`
	// Series is the series to associate with new state server machines.
	// If this is empty, then the environment's default series is used.
	Series string `json:"series,omitempty"`
	// Placement defines specific machines to become new state server machines.
	Placement []string `json:"placement,omitempty"`
}

// StateServersSpecs contains all the arguments
// for the EnsureAvailability API call.
type StateServersSpecs struct {
	Specs []StateServersSpec
}

// StateServersChangeResult contains the results
// of a single EnsureAvailability API call or
// an error.
type StateServersChangeResult struct {
	Result StateServersChanges
	Error  *Error
}

// StateServersChangeResults contains the results
// of the EnsureAvailability API call.
type StateServersChangeResults struct {
	Results []StateServersChangeResult
}

// StateServersChanges lists the servers
// that have been added, removed or maintained in the
// pool as a result of an ensure-availability operation.
type StateServersChanges struct {
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
