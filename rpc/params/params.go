// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/proxy"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/tools"
)

// Entity identifies a single entity.
type Entity struct {
	Tag string `json:"tag"`
}

// Entities identifies multiple entities.
type Entities struct {
	Entities []Entity `json:"entities"`
}

// EntitiesResults contains multiple Entities results (where each
// Entities is the result of a query).
type EntitiesResults struct {
	Results []EntitiesResult `json:"results"`
}

// EntitiesResult is the result of one query that either yields some
// set of entities or an error.
type EntitiesResult struct {
	Entities []Entity `json:"entities"`
	Error    *Error   `json:"error,omitempty"`
}

// EntityPasswords holds the parameters for making a SetPasswords call.
type EntityPasswords struct {
	Changes []EntityPassword `json:"changes"`
}

// EntityPassword specifies a password change for the entity
// with the given tag.
type EntityPassword struct {
	Tag      string `json:"tag"`
	Password string `json:"password"`
}

// ErrorResults holds the results of calling a bulk operation which
// returns no data, only an error result. The order and
// number of elements matches the operations specified in the request.
type ErrorResults struct {
	// Results contains the error results from each operation.
	Results []ErrorResult `json:"results"`
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
	Error *Error `json:"error,omitempty"`
}

// AddRelation holds the parameters for making the AddRelation call.
// The endpoints specified are unordered.
type AddRelation struct {
	Endpoints []string `json:"endpoints"`
	ViaCIDRs  []string `json:"via-cidrs,omitempty"`
}

// AddRelationResults holds the results of a AddRelation call. The Endpoints
// field maps application names to the involved endpoints.
type AddRelationResults struct {
	Endpoints map[string]CharmRelation `json:"endpoints"`
}

// DestroyRelation holds the parameters for making the DestroyRelation call.
// A relation is identified by either endpoints or id.
// The endpoints, if specified, are unordered.
type DestroyRelation struct {
	Endpoints  []string `json:"endpoints,omitempty"`
	RelationId int      `json:"relation-id"`

	// Force specifies whether relation destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in relation destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
}

// RelationStatusArgs holds the parameters for updating the status
// of one or more relations.
type RelationStatusArgs struct {
	Args []RelationStatusArg `json:"args"`
}

// RelationStatusArg holds the new status value for a relation.
type RelationStatusArg struct {
	UnitTag    string              `json:"unit-tag"`
	RelationId int                 `json:"relation-id"`
	Status     RelationStatusValue `json:"status"`
	Message    string              `json:"message"`
}

// RelationSuspendedArgs holds the parameters for setting
// the suspended status of one or more relations.
type RelationSuspendedArgs struct {
	Args []RelationSuspendedArg `json:"args"`
}

// RelationSuspendedArg holds the new suspended status value for a relation.
type RelationSuspendedArg struct {
	RelationId int    `json:"relation-id"`
	Message    string `json:"message"`
	Suspended  bool   `json:"suspended"`
}

// ProcessRelations holds the information required to process series of
// relations during a model migration.
type ProcessRelations struct {
	ControllerAlias string `json:"controller-alias"`
}

// AddCharm holds the arguments for making an AddCharm API call.
type AddCharm struct {
	URL     string `json:"url"`
	Channel string `json:"channel"`
	Force   bool   `json:"force"`
}

// AddCharmWithOrigin holds the arguments for making an AddCharm API call via the
// Charms facade.
type AddCharmWithOrigin struct {
	URL    string      `json:"url"`
	Origin CharmOrigin `json:"charm-origin"`
	Force  bool        `json:"force"`
}

// AddCharmWithAuthorization holds the arguments for making an
// AddCharmWithAuthorization API call.
type AddCharmWithAuthorization struct {
	URL                string             `json:"url"`
	Channel            string             `json:"channel"`
	CharmStoreMacaroon *macaroon.Macaroon `json:"macaroon"`
	Force              bool               `json:"force"`
}

// AddCharmWithAuth holds the arguments for making an
// AddCharmWithAuth API call via the Charms facade.
type AddCharmWithAuth struct {
	URL                string             `json:"url"`
	Origin             CharmOrigin        `json:"charm-origin"`
	CharmStoreMacaroon *macaroon.Macaroon `json:"macaroon"`
	Force              bool               `json:"force"`
}

// CharmOriginResult holds the results of AddCharms calls where
// a CharmOrigin was used.
type CharmOriginResult struct {
	Origin CharmOrigin `json:"charm-origin"`
	Error  *Error      `json:"error,omitempty"`
}

// CharmURLOriginResult holds the results of the charm's url and origin.
type CharmURLOriginResult struct {
	URL    string      `json:"url"`
	Origin CharmOrigin `json:"charm-origin"`
	Error  *Error      `json:"error,omitempty"`
}

// Base holds the name of an OS name and its version.
type Base struct {
	Name    string `json:"name"`
	Channel string `json:"channel"`
}

// AddMachineParams encapsulates the parameters used to create a new machine.
type AddMachineParams struct {
	// The following fields hold attributes that will be given to the
	// new machine when it is created.
	Base        *Base              `json:"base,omitempty"`
	Constraints constraints.Value  `json:"constraints"`
	Jobs        []model.MachineJob `json:"jobs"`

	// Disks describes constraints for disks that must be attached to
	// the machine when it is provisioned.
	Disks []storage.Directive `json:"disks,omitempty"`

	// If Placement is non-nil, it contains a placement directive
	// that will be used to decide how to instantiate the machine.
	Placement *instance.Placement `json:"placement,omitempty"`

	// If ParentId is non-empty, it specifies the id of the
	// parent machine within which the new machine will
	// be created. In that case, ContainerType must also be
	// set.
	ParentId string `json:"parent-id"`

	// ContainerType optionally gives the container type of the
	// new machine. If it is non-empty, the new machine
	// will be implemented by a container. If it is specified
	// but ParentId is empty, a new top level machine will
	// be created to hold the container with given series,
	// constraints and jobs.
	ContainerType instance.ContainerType `json:"container-type"`

	// If InstanceId is non-empty, it will be associated with
	// the new machine along with the given nonce,
	// hardware characteristics and addresses.
	// All the following fields will be ignored if ContainerType
	// is set.
	InstanceId              instance.Id                      `json:"instance-id"`
	Nonce                   string                           `json:"nonce"`
	HardwareCharacteristics instance.HardwareCharacteristics `json:"hardware-characteristics"`
	Addrs                   []Address                        `json:"addresses"`
}

// AddMachines holds the parameters for making the AddMachines call.
type AddMachines struct {
	MachineParams []AddMachineParams `json:"params"`
}

// AddMachinesResults holds the results of an AddMachines call.
type AddMachinesResults struct {
	Machines []AddMachinesResult `json:"machines"`
}

// AddMachinesResult holds the name of a machine added by the
// api.client.AddMachine call for a single machine.
type AddMachinesResult struct {
	Machine string `json:"machine"`
	Error   *Error `json:"error,omitempty"`
}

// DestroyMachinesParams holds parameters for the latest DestroyMachinesWithParams call.
type DestroyMachinesParams struct {
	MachineTags []string `json:"machine-tags"`
	Force       bool     `json:"force,omitempty"`
	Keep        bool     `json:"keep,omitempty"`
	DryRun      bool     `json:"dry-run,omitempty"`

	// MaxWait specifies the amount of time that each step in machine destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
}

// RecordAgentStartInformationArgs holds the parameters for updating the
// information reported by one or more machine agents when they start.
type RecordAgentStartInformationArgs struct {
	Args []RecordAgentStartInformationArg `json:"args"`
}

// RecordAgentStartInformationArg holds the information reported by a machine
// agnet to the controller when it starts.
type RecordAgentStartInformationArg struct {
	Tag      string `json:"tag"`
	Hostname string `json:"hostname,omitempty"`
}

// UpdateChannelArg holds the parameters for updating the series for the
// specified application or machine. For Application, only known by facade
// version 5 and greater. For MachineManger, only known by facade version
// 4 or greater.
type UpdateChannelArg struct {
	Entity  Entity `json:"tag"`
	Force   bool   `json:"force"`
	Channel string `json:"channel"`
}

// UpdateChannelArgs holds the parameters for updating the series
// of one or more applications or machines. For Application, only known
// by facade version 5 and greater. For MachineManger, only known by facade
// version 4 or greater.
type UpdateChannelArgs struct {
	Args []UpdateChannelArg `json:"args"`
}

// LXDProfileUpgrade holds the parameters for an application
// lxd profile machines
type LXDProfileUpgrade struct {
	Entities        []Entity `json:"entities"`
	ApplicationName string   `json:"application-name"`
}

// UpgradeCharmProfileStatusResult contains the lxd profile status result for an upgrading
// machine or container.
type UpgradeCharmProfileStatusResult struct {
	Error  *Error `json:"error,omitempty"`
	Status string `json:"status,omitempty"`
}

// UpgradeCharmProfileStatusResults contains the lxd profile status results for
// upgrading machines or container.
type UpgradeCharmProfileStatusResults struct {
	Results []UpgradeCharmProfileStatusResult `json:"results,omitempty"`
}

// ConfigResult holds configuration values for an entity.
type ConfigResult struct {
	Config map[string]interface{} `json:"config"`
	Error  *Error                 `json:"error,omitempty"`
}

// ModelOperatorInfo holds infor needed for a model operator.
type ModelOperatorInfo struct {
	APIAddresses []string          `json:"api-addresses"`
	ImageDetails DockerImageInfo   `json:"image-details"`
	Version      semversion.Number `json:"version"`
}

// IssueOperatorCertificateResult contains an x509 certificate
// for a CAAS Operator.
type IssueOperatorCertificateResult struct {
	CACert     string `json:"ca-cert"`
	Cert       string `json:"cert"`
	PrivateKey string `json:"private-key"`
	Error      *Error `json:"error,omitempty"`
}

// IssueOperatorCertificateResults holds IssueOperatorCertificate results.
type IssueOperatorCertificateResults struct {
	Results []IssueOperatorCertificateResult `json:"results"`
}

// PublicAddress holds parameters for the PublicAddress call.
type PublicAddress struct {
	Target string `json:"target"`
}

// PublicAddressResults holds results of the PublicAddress call.
type PublicAddressResults struct {
	PublicAddress string `json:"public-address"`
}

// PrivateAddress holds parameters for the PrivateAddress call.
type PrivateAddress struct {
	Target string `json:"target"`
}

// PrivateAddressResults holds results of the PrivateAddress call.
type PrivateAddressResults struct {
	PrivateAddress string `json:"private-address"`
}

// Resolved holds parameters for the Resolved call.
type Resolved struct {
	UnitName string `json:"unit-name"`
	Retry    bool   `json:"retry"`
}

// ResolvedResults holds results of the Resolved call.
type ResolvedResults struct {
	Application string                 `json:"application"`
	Charm       string                 `json:"charm"`
	Settings    map[string]interface{} `json:"settings"`
}

// UnitsResolved holds parameters for the ResolveUnitErrors call.
type UnitsResolved struct {
	Tags  Entities `json:"tags,omitempty"`
	Retry bool     `json:"retry,omitempty"`
	All   bool     `json:"all,omitempty"`
}

// AddApplicationUnitsResults holds the names of the units added by the
// AddUnits call.
type AddApplicationUnitsResults struct {
	Units []string `json:"units"`
}

// AddApplicationUnits holds parameters for the AddUnits call.
type AddApplicationUnits struct {
	ApplicationName string                `json:"application"`
	NumUnits        int                   `json:"num-units"`
	Placement       []*instance.Placement `json:"placement"`
	Policy          string                `json:"policy,omitempty"`
	AttachStorage   []string              `json:"attach-storage,omitempty"`
}

// AddApplicationUnitsV5 holds parameters for the AddUnits call.
// V5 is missing the new policy arg.
type AddApplicationUnitsV5 struct {
	ApplicationName string                `json:"application"`
	NumUnits        int                   `json:"num-units"`
	Placement       []*instance.Placement `json:"placement"`
	AttachStorage   []string              `json:"attach-storage,omitempty"`
}

// UpdateApplicationUnitArgs holds the parameters for
// updating application units.
type UpdateApplicationUnitArgs struct {
	Args []UpdateApplicationUnits `json:"args"`
}

// UpdateApplicationUnits holds unit parameters for a specified application.
type UpdateApplicationUnits struct {
	ApplicationTag string                  `json:"application-tag"`
	Scale          *int                    `json:"scale,omitempty"`
	Generation     *int64                  `json:"generation,omitempty"`
	Status         EntityStatus            `json:"status,omitempty"`
	Units          []ApplicationUnitParams `json:"units"`
}

// ApplicationUnitParams holds unit parameters used to update a unit.
type ApplicationUnitParams struct {
	ProviderId     string                     `json:"provider-id"`
	UnitTag        string                     `json:"unit-tag"`
	Address        string                     `json:"address"`
	Ports          []string                   `json:"ports"`
	Stateful       bool                       `json:"stateful,omitempty"`
	FilesystemInfo []KubernetesFilesystemInfo `json:"filesystem-info,omitempty"`
	Status         string                     `json:"status"`
	Info           string                     `json:"info"`
	Data           map[string]interface{}     `json:"data,omitempty"`
}

// UpdateApplicationUnitResults holds results from UpdateApplicationUnits
type UpdateApplicationUnitResults struct {
	Results []UpdateApplicationUnitResult `json:"results"`
}

// UpdateApplicationUnitResult holds a single result from UpdateApplicationUnits
type UpdateApplicationUnitResult struct {
	Info  *UpdateApplicationUnitsInfo `json:"info,omitempty"`
	Error *Error                      `json:"error,omitempty"`
}

// UpdateApplicationUnitsInfo holds info about the application units after a call to
// UpdateApplicationUnits
type UpdateApplicationUnitsInfo struct {
	Units []ApplicationUnitInfo `json:"units"`
}

// ApplicationUnitInfo holds info about the unit in the application.
type ApplicationUnitInfo struct {
	ProviderId string `json:"provider-id"`
	UnitTag    string `json:"unit-tag"`
}

// ApplicationMergeBindingsArgs holds the parameters for updating application
// bindings.
type ApplicationMergeBindingsArgs struct {
	Args []ApplicationMergeBindings `json:"args"`
}

// ApplicationMergeBindings holds a list of operator-defined bindings to be
// merged with the current application bindings.
type ApplicationMergeBindings struct {
	ApplicationTag string            `json:"application-tag"`
	Bindings       map[string]string `json:"bindings"`
	Force          bool              `json:"force"`
}

// DestroyUnitsParamsV15 holds bulk parameters for the Application.DestroyUnit call.
type DestroyUnitsParamsV15 struct {
	Units []DestroyUnitParamsV15 `json:"units"`
}

// DestroyUnitParams holds parameters for the Application.DestroyUnit call.
type DestroyUnitParamsV15 struct {
	// UnitTag holds the tag of the unit to destroy.
	UnitTag string `json:"unit-tag"`

	// DestroyStorage controls whether or not storage
	// attached to the unit should be destroyed.
	DestroyStorage bool `json:"destroy-storage,omitempty"`

	// Force controls whether or not the destruction of an application
	// will be forced, i.e. ignore operational errors.
	Force bool `json:"force"`

	// MaxWait specifies the amount of time that each step in unit removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
}

// DestroyApplicationUnits holds parameters for the deprecated
// Application.DestroyUnits call.
type DestroyApplicationUnits struct {
	UnitNames []string `json:"unit-names"`
}

// DestroyUnitsParams holds bulk parameters for the Application.DestroyUnit call.
type DestroyUnitsParams struct {
	Units []DestroyUnitParams `json:"units"`
}

// DestroyUnitParams holds parameters for the Application.DestroyUnit call.
type DestroyUnitParams struct {
	// UnitTag holds the tag of the unit to destroy.
	UnitTag string `json:"unit-tag"`

	// DestroyStorage controls whether or not storage
	// attached to the unit should be destroyed.
	DestroyStorage bool `json:"destroy-storage,omitempty"`

	// Force controls whether or not the destruction of an application
	// will be forced, i.e. ignore operational errors.
	Force bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in unit removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`

	// DryRun specifies whether to perform the destroy action or
	// just return what this action will destroy
	DryRun bool `json:"dry-run,omitempty"`
}

// Creds holds credentials for identifying an entity.
type Creds struct {
	AuthTag  string `json:"auth-tag"`
	Password string `json:"password"`
	Nonce    string `json:"nonce"`
}

// LoginRequest holds credentials for identifying an entity to the Login v1
// facade. AuthTag holds the tag of the user to connect as. If it is empty,
// then the provided macaroon slices will be used for authentication (if
// any one is valid, the authentication succeeds). If there are no
// valid macaroons and macaroon authentication is configured,
// the LoginResult will contain a macaroon that when
// discharged, may allow access.
type LoginRequest struct {
	AuthTag       string           `json:"auth-tag"`
	Credentials   string           `json:"credentials"`
	Nonce         string           `json:"nonce"`
	Macaroons     []macaroon.Slice `json:"macaroons"`
	BakeryVersion bakery.Version   `json:"bakery-version,omitempty"`

	// Token represents a JSON Web Token (JWT).
	Token         string `json:"token,omitempty"`
	CLIArgs       string `json:"cli-args,omitempty"`
	UserData      string `json:"user-data"`
	ClientVersion string `json:"client-version,omitempty"`
}

// LoginRequestCompat holds credentials for identifying an entity to the Login v1
// or earlier (v0 or even pre-facade).
type LoginRequestCompat struct {
	LoginRequest `json:"login-request"`
	Creds        `json:"creds"`
}

// GetAnnotationsResults holds annotations associated with an entity.
type GetAnnotationsResults struct {
	Annotations map[string]string `json:"annotations"`
}

// GetAnnotations stores parameters for making the GetAnnotations call.
type GetAnnotations struct {
	Tag string `json:"tag"`
}

// SetAnnotations stores parameters for making the SetAnnotations call.
type SetAnnotations struct {
	Tag   string            `json:"tag"`
	Pairs map[string]string `json:"annotations"`
}

// GetConstraintsResults holds results of the GetConstraints call.
type GetConstraintsResults struct {
	Constraints constraints.Value `json:"constraints"`
}

// SetConstraints stores parameters for making the SetConstraints call.
type SetConstraints struct {
	ApplicationName string            `json:"application"` //optional, if empty, model constraints are set.
	Constraints     constraints.Value `json:"constraints"`
}

// ResolveCharms stores charm references for a ResolveCharms call.
type ResolveCharms struct {
	References []string `json:"references"`
}

// ResolveCharmResult holds the result of resolving a charm reference to a URL, or any error that occurred.
type ResolveCharmResult struct {
	// URL is a string representation of charm.URL.
	URL   string `json:"url,omitempty"`
	Error string `json:"error,omitempty"`
}

// ResolveCharmResults holds results of the ResolveCharms call.
type ResolveCharmResults struct {
	URLs []ResolveCharmResult `json:"urls"`
}

// ResolveCharmWithChannel contains a charm reference with the desired
// channel to be resolved.
type ResolveCharmWithChannel struct {
	Reference string      `json:"reference"`
	Origin    CharmOrigin `json:"charm-origin"`

	// SwitchCharm is set to true when the purpose of this resolve request
	// is to switch a different charm (potentially from a different store).
	SwitchCharm bool `json:"switch-charm,omitempty"`
}

// ResolveCharmsWithChannel contains of slice of data on charms to be
// resolved.
type ResolveCharmsWithChannel struct {
	Resolve  []ResolveCharmWithChannel `json:"resolve"`
	Macaroon *macaroon.Macaroon        `json:"macaroon,omitempty"`
}

// ResolveCharmWithChannelResult is the result of a single charm resolution.
type ResolveCharmWithChannelResult struct {
	URL            string      `json:"url"`
	Origin         CharmOrigin `json:"charm-origin"`
	SupportedBases []Base      `json:"supported-bases"`
	Error          *Error      `json:"error,omitempty"`
}

// ResolveCharmWithChannelResults holds the results of ResolveCharmsWithChannel.
type ResolveCharmWithChannelResults struct {
	Results []ResolveCharmWithChannelResult
}

// CharmURLAndOrigins contains a slice of charm urls with a given origin.
type CharmURLAndOrigins struct {
	Entities []CharmURLAndOrigin `json:"entities"`
}

// CharmURLAndOrigin holds the information for selecting one bundle
type CharmURLAndOrigin struct {
	CharmURL string             `json:"charm-url"`
	Origin   CharmOrigin        `json:"charm-origin"`
	Macaroon *macaroon.Macaroon `json:"macaroon,omitempty"`
}

// DownloadInfoResults returns the download url for a given request.
type DownloadInfoResults struct {
	Results []DownloadInfoResult `json:"results"`
}

// DownloadInfoResult returns a given bundle for a request.
type DownloadInfoResult struct {
	URL    string      `json:"url"`
	Origin CharmOrigin `json:"charm-origin"`
}

// AllWatcherId holds the id of an AllWatcher.
type AllWatcherId struct {
	AllWatcherId string `json:"watcher-id"`
}

// SSHListMode describes the mode to use when list ssh keys. This value has been
// brought over from [github.com/juju/utils/ssh].
type SSHListMode bool

const (
	// SSHListModeFull is a [SSHListMode] that list ssh keys with their full raw
	// value. This const comes from [github.com/juju/utils/ssh.FullKeys]
	SSHListModeFull = SSHListMode(true)

	// SSHListModeFingerprint is a [SSHListMode] that list ssh keys with just
	// their fingerprint value. This const comes from
	// [github.com/juju/utils/ssh.Fingerprints]
	SSHListModeFingerprint = SSHListMode(false)
)

// ListSSHKeys stores parameters used for a KeyManager.ListKeys call.
type ListSSHKeys struct {
	Entities `json:"entities"`
	Mode     SSHListMode `json:"mode"`
}

// ModifyUserSSHKeys stores parameters used for a KeyManager.Add|Delete|Import call for a user.
type ModifyUserSSHKeys struct {
	User string   `json:"user"`
	Keys []string `json:"ssh-keys"`
}

// StateServingInfo holds information needed by a state
// server.
type StateServingInfo struct {
	APIPort   int `json:"api-port"`
	StatePort int `json:"state-port"`
	// The controller cert and corresponding private key.
	Cert       string `json:"cert"`
	PrivateKey string `json:"private-key"`
	// The private key for the CA cert so that a new controller
	// cert can be generated when needed.
	CAPrivateKey string `json:"ca-private-key"`
	// this will be passed as the KeyFile argument to MongoDB
	SystemIdentity string `json:"system-identity"`

	// Deprecated: ControllerAPIPort is no longer used.
	ControllerAPIPort int `json:"controller-api-port,omitempty"`
}

// IsMasterResult holds the result of an IsMaster API call.
type IsMasterResult struct {
	// Master reports whether the connected agent
	// lives on the same instance as the mongo replica
	// set master.
	Master bool `json:"master"`
}

// ContainerManagerConfigParams contains the parameters for the
// ContainerManagerConfig provisioner API call.
type ContainerManagerConfigParams struct {
	Type instance.ContainerType `json:"type"`
}

// ContainerManagerConfig contains information from the model config
// that is needed for configuring the container manager.
type ContainerManagerConfig struct {
	ManagerConfig map[string]string `json:"config"`
}

// UpdateBehavior contains settings that are duplicated in several
// places. Let's just embed this instead.
type UpdateBehavior struct {
	EnableOSRefreshUpdate bool `json:"enable-os-refresh-update"`
	EnableOSUpgrade       bool `json:"enable-os-upgrade"`
}

// ContainerConfig contains information from the model config that is
// needed for container cloud-init.
type ContainerConfig struct {
	ProviderType               string                 `json:"provider-type"`
	AuthorizedKeys             string                 `json:"authorized-keys"`
	SSLHostnameVerification    bool                   `json:"ssl-hostname-verification"`
	LegacyProxy                proxy.Settings         `json:"legacy-proxy"`
	JujuProxy                  proxy.Settings         `json:"juju-proxy"`
	AptProxy                   proxy.Settings         `json:"apt-proxy"`
	SnapProxy                  proxy.Settings         `json:"snap-proxy"`
	SnapStoreAssertions        string                 `json:"snap-store-assertions"`
	SnapStoreProxyID           string                 `json:"snap-store-proxy-id"`
	SnapStoreProxyURL          string                 `json:"snap-store-proxy-url"`
	AptMirror                  string                 `json:"apt-mirror,omitempty"`
	CloudInitUserData          map[string]interface{} `json:"cloudinit-userdata,omitempty"`
	ContainerInheritProperties string                 `json:"container-inherit-properties,omitempty"`
	*UpdateBehavior
}

// ProvisioningScriptParams contains the parameters for the
// ProvisioningScript client API call.
type ProvisioningScriptParams struct {
	MachineId string `json:"machine-id"`
	Nonce     string `json:"nonce"`

	// DataDir may be "", in which case the default will be used.
	DataDir string `json:"data-dir"`

	// DisablePackageCommands may be set to disable all
	// package-related commands. It is then the responsibility of the
	// provisioner to ensure that all the packages required by Juju
	// are available.
	DisablePackageCommands bool `json:"disable-package-commands"`
}

// ProvisioningScriptResult contains the result of the
// ProvisioningScript client API call.
type ProvisioningScriptResult struct {
	Script string `json:"script"`
}

// DeployerConnectionValues containers the result of deployer.ConnectionInfo
// API call.
type DeployerConnectionValues struct {
	APIAddresses []string `json:"api-addresses"`
}

// IsControllerResult holds the result of an IsController call, which returns
// whether a given machine is a controller machine.
type IsControllerResult struct {
	IsController bool   `json:"is-controller"`
	Error        *Error `json:"error,omitempty"`
}

// IsControllerResults holds the result of a call to IsController
type IsControllerResults struct {
	Results []IsControllerResult `json:"results"`
}

// JobsResult holds the jobs for a machine that are returned by a call to Jobs.
// Deprecated: Jobs is being deprecated. Use IsController instead.
type JobsResult struct {
	Jobs  []string `json:"jobs"`
	Error *Error   `json:"error,omitempty"`
}

// JobsResults holds the result of a call to Jobs.
type JobsResults struct {
	Results []JobsResult `json:"results"`
}

// DistributionGroupResult contains the result of
// the DistributionGroup provisioner API call.
type DistributionGroupResult struct {
	Error  *Error        `json:"error,omitempty"`
	Result []instance.Id `json:"result"`
}

// DistributionGroupResults is the bulk form of
// DistributionGroupResult.
type DistributionGroupResults struct {
	Results []DistributionGroupResult `json:"results"`
}

// FacadeVersions describes the available Facades and what versions of each one
// are available
type FacadeVersions struct {
	Name     string `json:"name"`
	Versions []int  `json:"versions"`
}

// RedirectInfoResult holds the result of a RedirectInfo call.
type RedirectInfoResult struct {
	// Servers holds an entry for each server that holds the
	// addresses for the server.
	Servers [][]HostPort `json:"servers"`

	// CACert holds the CA certificate for the server.
	// TODO(rogpeppe) allow this to be empty if the
	// server has a globally trusted certificate?
	CACert string `json:"ca-cert"`
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

	// ControllerAccess holds the access the user has to the connected controller.
	// It will be empty if the user has no access to the controller.
	ControllerAccess string `json:"controller-access"`

	// ModelAccess holds the access the user has to the connected model.
	ModelAccess string `json:"model-access"`
}

// LoginResult holds the result of an Admin Login call.
type LoginResult struct {
	// DischargeRequired implies that the login request has failed, and none of
	// the other fields are populated. It contains a macaroon which, when
	// discharged, will grant access on a subsequent call to Login.
	// Note: It is OK to use the Macaroon type here as it is explicitly
	// designed to provide stable serialisation of macaroons.  It's good
	// practice to only use primitives in types that will be serialised,
	// however because of the above it is suitable to use the Macaroon type
	// here.
	DischargeRequired *macaroon.Macaroon `json:"discharge-required,omitempty"`

	// BakeryDischargeRequired implies that the login request has failed, and none of
	// the other fields are populated. It contains a macaroon which, when
	// discharged, will grant access on a subsequent call to Login.
	// Note: It is OK to use the Macaroon type here as it is explicitly
	// designed to provide stable serialisation of macaroons.  It's good
	// practice to only use primitives in types that will be serialised,
	// however because of the above it is suitable to use the Macaroon type
	// here.
	// This is the macaroon emitted by newer Juju controllers using bakery.v2.
	BakeryDischargeRequired *bakery.Macaroon `json:"bakery-discharge-required,omitempty"`

	// DischargeRequiredReason holds the reason that the above discharge was
	// required.
	DischargeRequiredReason string `json:"discharge-required-error,omitempty"`

	// Servers is the list of API server addresses.
	Servers [][]HostPort `json:"servers,omitempty"`

	// PublicDNSName holds the host name for which an officially
	// signed certificate will be used for TLS connection to the server.
	// If empty, the private Juju CA certificate must be used to verify
	// the connection.
	PublicDNSName string `json:"public-dns-name,omitempty"`

	// ModelTag is the tag for the model that is being connected to.
	ModelTag string `json:"model-tag,omitempty"`

	// ControllerTag is the tag for the controller that runs the API servers.
	ControllerTag string `json:"controller-tag,omitempty"`

	// UserInfo describes the authenticated user, if any.
	UserInfo *AuthUserInfo `json:"user-info,omitempty"`

	// Facades describes all the available API facade versions to the
	// authenticated client.
	Facades []FacadeVersions `json:"facades,omitempty"`

	// ServerVersion is the string representation of the server version
	// if the server supports it.
	ServerVersion string `json:"server-version,omitempty"`
}

// ControllersSpec contains arguments for
// the EnableHA client API call.
type ControllersSpec struct {
	NumControllers int               `json:"num-controllers"`
	Constraints    constraints.Value `json:"constraints,omitempty"`
	// Placement defines specific machines to become new controller machines.
	Placement []string `json:"placement,omitempty"`
}

// ControllersSpecs contains all the arguments
// for the EnableHA API call.
type ControllersSpecs struct {
	Specs []ControllersSpec `json:"specs"`
}

// ControllerDetailsResults contains the results
// of a call to fetch controller config details.
type ControllerDetailsResults struct {
	Results []ControllerDetails `json:"results"`
}

// ControllerDetails contains the details about a controller.
type ControllerDetails struct {
	ControllerId string   `json:"controller-id"`
	APIAddresses []string `json:"api-addresses"`
	Error        *Error   `json:"error,omitempty"`
}

// FindToolsParams defines parameters for the FindTools method.
type FindToolsParams struct {
	// Number will be used to match tools versions exactly if non-zero.
	Number semversion.Number `json:"number"`

	// MajorVersion will be used to match the major version if non-zero.
	// TODO(juju 3.1) - remove
	MajorVersion int `json:"major"`

	// Arch will be used to match tools by architecture if non-empty.
	Arch string `json:"arch"`

	// OSType will be used to match tools by os type if non-empty.
	OSType string `json:"os-type"`

	// AgentStream will be used to set agent stream to search
	AgentStream string `json:"agentstream"`
}

// FindToolsResult holds a list of tools from FindTools and any error.
type FindToolsResult struct {
	List  tools.List `json:"list"`
	Error *Error     `json:"error,omitempty"`
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
// The log messages are sent by the log sender worker, used by agents to
// send logs to the controller.
type LogRecord struct {
	Time     time.Time         `json:"t"`
	Module   string            `json:"m"`
	Location string            `json:"l"`
	Level    string            `json:"v"`
	Message  string            `json:"x"`
	Entity   string            `json:"e,omitempty"`
	Labels   map[string]string `json:"b,omitempty"`
}

type logRecordJSON struct {
	Time     time.Time `json:"t"`
	Module   string    `json:"m"`
	Location string    `json:"l"`
	Level    string    `json:"v"`
	Message  string    `json:"x"`
	Entity   string    `json:"e,omitempty"`
	Labels   any       `json:"b,omitempty"`
}

// UnmarshalJSON unmarshalls an incoming log record
// which may have been generated by an older client.
func (m *LogRecord) UnmarshalJSON(data []byte) error {
	var jr logRecordJSON
	if err := json.Unmarshal(data, &jr); err != nil {
		return errors.Trace(err)
	}
	m.Time = jr.Time
	m.Entity = jr.Entity
	m.Level = jr.Level
	m.Module = jr.Module
	m.Location = jr.Location
	m.Message = jr.Message
	m.Labels = unmarshallLogLabels(jr.Labels)
	return nil
}

// PubSubMessage is used to propagate pubsub messages from one api server to the
// others.
type PubSubMessage struct {
	Topic string                 `json:"topic"`
	Data  map[string]interface{} `json:"data"`
}

// LeaseOperations is used to send raft operational messages between controllers.
type LeaseOperations struct {
	Operations []LeaseOperation `json:"commands"`
}

// LeaseOperation is used to send raft operational messages between controllers.
type LeaseOperation struct {
	Command string `json:"command"`
}

// LeaseOperationsV2 is used to send raft operational messages between
// controllers.
type LeaseOperationsV2 struct {
	Operations []LeaseOperationCommand `json:"commands"`
}

// LeaseOperationCommand is used to send raft operational messages between
// controllers.
type LeaseOperationCommand struct {
	// Version of the command format in case it changes,
	// and we need to handle multiple formats.
	Version int `json:"version"`

	// Operation is one of claim, extend, expire or setTime.
	Operation string `json:"operation"`

	// Namespace is the kind of lease.
	Namespace string `json:"namespace,omitempty"`

	// ModelUUID identifies the model the lease belongs to.
	ModelUUID string `json:"model-uuid,omitempty"`

	// Lease is the name of the lease the command affects.
	Lease string `json:"lease,omitempty"`

	// Holder is the name of the party claiming or extending the
	// lease.
	Holder string `json:"holder,omitempty"`

	// Duration is how long the lease should last.
	Duration time.Duration `json:"duration,omitempty"`

	// OldTime is the previous time for time updates (to avoid
	// applying stale ones).
	OldTime time.Time `json:"old-time,omitempty"`

	// NewTime is the time to store as the global time.
	NewTime time.Time `json:"new-time,omitempty"`

	// PinEntity is a tag representing an entity concerned
	// with a pin or unpin operation.
	PinEntity string `json:"pin-entity,omitempty"`
}

// ExportBundleParams holds parameters for exporting Bundles.
type ExportBundleParams struct {
	IncludeCharmDefaults bool `json:"include-charm-defaults,omitempty"`
}

// BundleChangesParams holds parameters for making Bundle.GetChanges calls.
type BundleChangesParams struct {
	// BundleDataYAML is the YAML-encoded charm bundle data
	// (see "github.com/juju/charm.BundleData").
	BundleDataYAML string `json:"yaml"`
	BundleURL      string `json:"bundleURL"`
}

// BundleChangesMapArgsResults holds results of the Bundle.GetChanges call.
type BundleChangesMapArgsResults struct {
	// Changes holds the list of changes required to deploy the bundle.
	// It is omitted if the provided bundle YAML has verification errors.
	Changes []*BundleChangesMapArgs `json:"changes,omitempty"`
	// Errors holds possible bundle verification errors.
	Errors []string `json:"errors,omitempty"`
}

// BundleChangesMapArgs holds a single change required to deploy a bundle.
// BundleChangesMapArgs has Args represented as a map of arguments rather
// than a series.
type BundleChangesMapArgs struct {
	// Id is the unique identifier for this change.
	Id string `json:"id"`
	// Method is the action to be performed to apply this change.
	Method string `json:"method"`
	// Args holds a list of arguments to pass to the method.
	Args map[string]interface{} `json:"args"`
	// Requires holds a list of dependencies for this change. Each dependency
	// is represented by the corresponding change id, and must be applied
	// before this change is applied.
	Requires []string `json:"requires"`
}

type MongoVersion struct {
	Major         int    `json:"major"`
	Minor         int    `json:"minor"`
	Patch         string `json:"patch"`
	StorageEngine string `json:"engine"`
}

// MacaroonResults contains a set of MacaroonResults.
type MacaroonResults struct {
	Results []MacaroonResult `json:"results"`
}

// MacaroonResult contains a macaroon or an error.
type MacaroonResult struct {
	Result *macaroon.Macaroon `json:"result,omitempty"`
	Error  *Error             `json:"error,omitempty"`
}

// DestroyMachineResults contains the results of a MachineManager.Destroy
// API request.
type DestroyMachineResults struct {
	Results []DestroyMachineResult `json:"results,omitempty"`
}

// DestroyMachineResult contains one of the results of a MachineManager.Destroy
// API request.
type DestroyMachineResult struct {
	Error *Error              `json:"error,omitempty"`
	Info  *DestroyMachineInfo `json:"info,omitempty"`
}

// DestroyMachineInfo contains information related to the removal of
// a machine.
type DestroyMachineInfo struct {
	// MachineId is the ID if the machine that will be destroyed
	MachineId string `json:"machine-id"`

	// DetachedStorage is the tags of storage instances that will be
	// detached from the machine (assigned units) as a result of
	// destroying the machine, and will remain in the model after
	// the machine and unit are removed.
	DetachedStorage []Entity `json:"detached-storage,omitempty"`

	// DestroyedStorage is the tags of storage instances that will be
	// destroyed as a result of destroying the machine.
	DestroyedStorage []Entity `json:"destroyed-storage,omitempty"`

	// DestroyedUnits are the tags of units that will be destroyed
	// as a result of destroying the machine.
	DestroyedUnits []Entity `json:"destroyed-units,omitempty"`

	// DestroyedContainers are the results of the destroyed containers hosted
	// on a machine, destroyed as a result of destroying the machine
	DestroyedContainers []DestroyMachineResult `json:"destroyed-containers,omitempty"`
}

// DestroyUnitResults contains the results of a DestroyUnit API request.
type DestroyUnitResults struct {
	Results []DestroyUnitResult `json:"results,omitempty"`
}

// DestroyUnitResult contains one of the results of a
// DestroyUnit API request.
type DestroyUnitResult struct {
	Error *Error           `json:"error,omitempty"`
	Info  *DestroyUnitInfo `json:"info,omitempty"`
}

// DestroyUnitInfo contains information related to the removal of
// an application unit.
type DestroyUnitInfo struct {
	// DetachedStorage is the tags of storage instances that will be
	// detached from the unit, and will remain in the model after
	// the unit is removed.
	DetachedStorage []Entity `json:"detached-storage,omitempty"`

	// DestroyedStorage is the tags of storage instances that will be
	// destroyed as a result of destroying the unit.
	DestroyedStorage []Entity `json:"destroyed-storage,omitempty"`
}

// DumpModelRequest wraps the request for a dump-model call.
// A simplified dump will not contain a complete export, but instead
// a reduced set that is determined by the server.
type DumpModelRequest struct {
	Entities   []Entity `json:"entities"`
	Simplified bool     `json:"simplified"`
}

type ProfileArg struct {
	Entity   Entity `json:"entity"`
	UnitName string `json:"unit-name"`
}

type ProfileArgs struct {
	Args []ProfileArg `json:"args"`
}

type ProfileInfoResult struct {
	ApplicationName string           `json:"application-name,omitempty"`
	Revision        int              `json:"revision,omitempty"`
	Profile         *CharmLXDProfile `json:"profile,omitempty"`
	Error           *Error           `json:"error,omitempty"`
}

type ProfileChangeResult struct {
	OldProfileName string           `json:"old-profile-name,omitempty"`
	NewProfileName string           `json:"new-profile-name,omitempty"`
	Profile        *CharmLXDProfile `json:"profile,omitempty"`
	Subordinate    bool             `json:"subordinate,omitempty"`
	Error          *Error           `json:"error,omitempty"`
}

type ProfileChangeResults struct {
	Results []ProfileChangeResult `json:"results"`
}

type SetProfileArgs struct {
	Args []SetProfileArg `json:"args"`
}

type SetProfileArg struct {
	Entity   Entity   `json:"entity"`
	Profiles []string `json:"profiles"`
}

type SetProfileUpgradeCompleteArgs struct {
	Args []SetProfileUpgradeCompleteArg `json:"args"`
}

type SetProfileUpgradeCompleteArg struct {
	Entity   Entity `json:"entity"`
	UnitName string `json:"unit-name"`
	Message  string `json:"message"`
}

// BranchArg represents an in-flight branch via its model and branch name.
type BranchArg struct {
	BranchName string `json:"branch"`
}

// GenerationId represents an GenerationId from a branch.
type GenerationId struct {
	GenerationId int `json:"generation-id"`
}

// BranchInfoArgs transports arguments to the BranchInfo method
type BranchInfoArgs struct {
	// BranchNames is the names of branches for which info is being requested.
	BranchNames []string `json:"branches"`

	// Detailed indicates whether full unit tracking detail should returned,
	// or a summary.
	Detailed bool `json:"detailed"`
}

// BranchTrackArg identifies an in-flight branch and a collection of
// entities that should be set to track changes made under the branch.
type BranchTrackArg struct {
	BranchName string   `json:"branch"`
	Entities   []Entity `json:"entities"`
	NumUnits   int      `json:"num-units,omitempty"`
}

// GenerationApplication represents changes to an application
// made under a branch.
type GenerationApplication struct {
	// ApplicationsName is the name of the application.
	ApplicationName string `json:"application"`

	// UnitProgress is summary information about units tracking the branch.
	UnitProgress string `json:"progress"`

	// UnitsTracking is the names of application units that have been set to
	// track the branch.
	UnitsTracking []string `json:"tracking,omitempty"`

	// UnitsPending is the names of application units that are still tracking
	// the master generation.
	UnitsPending []string `json:"pending,omitempty"`

	// Config changes are the effective new configuration values resulting from
	// changes made under this branch.
	ConfigChanges map[string]interface{} `json:"config"`
}

// Generation represents a model generation's details including config changes.
type Generation struct {
	// BranchName uniquely identifies a branch *amongst in-flight branches*.
	BranchName string `json:"branch"`

	// Created is the Unix timestamp at generation creation.
	Created int64 `json:"created"`

	// Created is the user who created the generation.
	CreatedBy string `json:"created-by"`

	// Completed is the Unix timestamp at generation completion/commit.
	Completed int64 `json:"completed,omitempty"`

	// CompletedBy is the user who committed/completed the generation.
	CompletedBy string `json:"completed-by,omitempty"`

	// GenerationId is the id .
	GenerationId int `json:"generation-id,omitempty"`

	// Applications holds the collection of application changes
	// made under this generation.
	Applications []GenerationApplication `json:"applications"`
}

// BranchResults transports a collection of generation details.
type BranchResults struct {
	// Generations holds the details of the requested generations.
	Generations []Generation `json:"generations"`

	// Error holds the value of any error that occurred processing the request.
	Error *Error `json:"error,omitempty"`
}

// GenerationResult transports a generation detail.
type GenerationResult struct {
	// Generation holds the details of the requested generation.
	Generation Generation `json:"generation"`

	// Error holds the value of any error that occurred processing the request.
	Error *Error `json:"error,omitempty"`
}

// CharmProfilingInfoResult contains the result based on ProfileInfoArg values
// to update profiles on a machine.
type CharmProfilingInfoResult struct {
	InstanceId      instance.Id         `json:"instance-id"`
	ModelName       string              `json:"model-name"`
	ProfileChanges  []ProfileInfoResult `json:"profile-changes"`
	CurrentProfiles []string            `json:"current-profiles"`
	Error           *Error              `json:"error"`
}

// WatchContainerStartArg contains arguments for watching for container start
// events on a CAAS application.
type WatchContainerStartArg struct {
	Entity    Entity `json:"entity"`
	Container string `json:"container,omitempty"`
}

// WatchContainerStartArgs holds the details to watch many containers for start
// events.
type WatchContainerStartArgs struct {
	Args []WatchContainerStartArg `json:"args"`
}

// CLICommands holds credentials, model and a list of CLI commands to run.
type CLICommands struct {
	User        string           `json:"user"`
	Credentials string           `json:"credentials,omitempty"`
	Macaroons   []macaroon.Slice `json:"macaroons,omitempty"`

	ActiveBranch string   `json:"active-branch,omitempty"`
	Commands     []string `json:"commands"`
}

// CLICommandStatus represents a status update for a CLI command.
type CLICommandStatus struct {
	Output []string `json:"output,omitempty"`
	Done   bool     `json:"done,omitempty"`
	Error  *Error   `json:"error,omitempty"`
}

// VirtualHostnameTargetArg holds the target tag and an optional container
// name to resolve a virtual hostname.
type VirtualHostnameTargetArg struct {
	Tag       string  `json:"tag"`
	Container *string `json:"container,omitempty"`
}
