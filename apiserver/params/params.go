// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/replicaset"
	"github.com/juju/utils/ssh"
	"github.com/juju/version"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/tools"
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

// AddCharm holds the arguments for making an AddCharm API call.
type AddCharm struct {
	URL     string `json:"url"`
	Channel string `json:"channel"`
	Force   bool   `json:"force"`
}

// AddCharmWithAuthorization holds the arguments for making an AddCharmWithAuthorization API call.
type AddCharmWithAuthorization struct {
	URL                string             `json:"url"`
	Channel            string             `json:"channel"`
	CharmStoreMacaroon *macaroon.Macaroon `json:"macaroon"`
	Force              bool               `json:"force"`
}

// AddMachineParams encapsulates the parameters used to create a new machine.
type AddMachineParams struct {
	// The following fields hold attributes that will be given to the
	// new machine when it is created.
	Series      string                    `json:"series"`
	Constraints constraints.Value         `json:"constraints"`
	Jobs        []multiwatcher.MachineJob `json:"jobs"`

	// Disks describes constraints for disks that must be attached to
	// the machine when it is provisioned.
	Disks []storage.Constraints `json:"disks,omitempty"`

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

// DestroyMachines holds parameters for the DestroyMachines call.
// This is the legacy params struct used with the client facade.
// TODO(wallyworld) - remove in Juju 3.0
type DestroyMachines struct {
	MachineNames []string `json:"machine-names"`
	Force        bool     `json:"force"`
}

// DestroyMachinesParams holds parameters for the DestroyMachinesWithParams call.
type DestroyMachinesParams struct {
	MachineTags []string `json:"machine-tags"`
	Force       bool     `json:"force,omitempty"`
	Keep        bool     `json:"keep,omitempty"`
}

// ApplicationsDeploy holds the parameters for deploying one or more applications.
type ApplicationsDeploy struct {
	Applications []ApplicationDeploy `json:"applications"`
}

// ApplicationDeploy holds the parameters for making the application Deploy call.
type ApplicationDeploy struct {
	ApplicationName  string                         `json:"application"`
	Series           string                         `json:"series"`
	CharmURL         string                         `json:"charm-url"`
	Channel          string                         `json:"channel"`
	NumUnits         int                            `json:"num-units"`
	Config           map[string]string              `json:"config,omitempty"`
	ConfigYAML       string                         `json:"config-yaml"` // Takes precedence over config if both are present.
	Constraints      constraints.Value              `json:"constraints"`
	Placement        []*instance.Placement          `json:"placement,omitempty"`
	Policy           string                         `json:"policy,omitempty"`
	Storage          map[string]storage.Constraints `json:"storage,omitempty"`
	Devices          map[string]devices.Constraints `json:"devices,omitempty"`
	AttachStorage    []string                       `json:"attach-storage,omitempty"`
	EndpointBindings map[string]string              `json:"endpoint-bindings,omitempty"`
	Resources        map[string]string              `json:"resources,omitempty"`
}

// ApplicationsDeployV5 holds the parameters for deploying one or more applications.
type ApplicationsDeployV5 struct {
	Applications []ApplicationDeployV5 `json:"applications"`
}

// ApplicationDeployV5 holds the parameters for making the application Deploy call for
// application facades older than v6. Missing the newer Policy arg.
type ApplicationDeployV5 struct {
	ApplicationName  string                         `json:"application"`
	Series           string                         `json:"series"`
	CharmURL         string                         `json:"charm-url"`
	Channel          string                         `json:"channel"`
	NumUnits         int                            `json:"num-units"`
	Config           map[string]string              `json:"config,omitempty"`
	ConfigYAML       string                         `json:"config-yaml"` // Takes precedence over config if both are present.
	Constraints      constraints.Value              `json:"constraints"`
	Placement        []*instance.Placement          `json:"placement,omitempty"`
	Storage          map[string]storage.Constraints `json:"storage,omitempty"`
	AttachStorage    []string                       `json:"attach-storage,omitempty"`
	EndpointBindings map[string]string              `json:"endpoint-bindings,omitempty"`
	Resources        map[string]string              `json:"resources,omitempty"`
}

// ApplicationsDeployV6 holds the parameters for deploying one or more applications.
type ApplicationsDeployV6 struct {
	Applications []ApplicationDeployV6 `json:"applications"`
}

// ApplicationDeployV6 holds the parameters for making the application Deploy call for
// application facades older than v6. Missing the newer Device arg.
type ApplicationDeployV6 struct {
	ApplicationName  string                         `json:"application"`
	Series           string                         `json:"series"`
	CharmURL         string                         `json:"charm-url"`
	Channel          string                         `json:"channel"`
	NumUnits         int                            `json:"num-units"`
	Config           map[string]string              `json:"config,omitempty"`
	ConfigYAML       string                         `json:"config-yaml"` // Takes precedence over config if both are present.
	Constraints      constraints.Value              `json:"constraints"`
	Placement        []*instance.Placement          `json:"placement,omitempty"`
	Policy           string                         `json:"policy,omitempty"`
	Storage          map[string]storage.Constraints `json:"storage,omitempty"`
	AttachStorage    []string                       `json:"attach-storage,omitempty"`
	EndpointBindings map[string]string              `json:"endpoint-bindings,omitempty"`
	Resources        map[string]string              `json:"resources,omitempty"`
}

// ApplicationUpdate holds the parameters for making the application Update call.
type ApplicationUpdate struct {
	ApplicationName string             `json:"application"`
	CharmURL        string             `json:"charm-url"`
	ForceCharmURL   bool               `json:"force-charm-url"`
	ForceSeries     bool               `json:"force-series"`
	MinUnits        *int               `json:"min-units,omitempty"`
	SettingsStrings map[string]string  `json:"settings,omitempty"`
	SettingsYAML    string             `json:"settings-yaml"` // Takes precedence over SettingsStrings if both are present.
	Constraints     *constraints.Value `json:"constraints,omitempty"`
}

// UpdateSeriesArg holds the parameters for updating the series for the
// specified application or machine. For Application, only known by facade
// version 5 and greater. For MachineManger, only known by facade version
// 4 or greater.
type UpdateSeriesArg struct {
	Entity Entity `json:"tag"`
	Force  bool   `json:"force"`
	Series string `json:"series"`
}

// UpdateSeriesArgs holds the parameters for updating the series
// of one or more applications or machines. For Application, only known
// by facade version 5 and greater. For MachineManger, only known by facade
// version 4 or greater.
type UpdateSeriesArgs struct {
	Args []UpdateSeriesArg `json:"args"`
}

// ApplicationSetCharm sets the charm for a given application.
type ApplicationSetCharm struct {
	// ApplicationName is the name of the application to set the charm on.
	ApplicationName string `json:"application"`

	// CharmURL is the new url for the charm.
	CharmURL string `json:"charm-url"`

	// Channel is the charm store channel from which the charm came.
	Channel string `json:"channel"`

	// ConfigSettings is the charm settings to set during the upgrade.
	// This field is only understood by Application facade version 2
	// and greater.
	ConfigSettings map[string]string `json:"config-settings,omitempty"`

	// ConfigSettingsYAML is the charm settings in YAML format to set
	// during the upgrade. If this is non-empty, it will take precedence
	// over ConfigSettings. This field is only understood by Application
	// facade version 2
	ConfigSettingsYAML string `json:"config-settings-yaml,omitempty"`

	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool `json:"force-units"`

	// ForceSeries forces the use of the charm even if it doesn't match the
	// series of the unit.
	ForceSeries bool `json:"force-series"`

	// ResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	ResourceIDs map[string]string `json:"resource-ids,omitempty"`

	// StorageConstraints is a map of storage names to storage constraints to
	// update during the upgrade. This field is only understood by Application
	// facade version 2 and greater.
	StorageConstraints map[string]StorageConstraints `json:"storage-constraints,omitempty"`
}

// ApplicationSetCharmProfile holds the parameters for making the
// application SetCharmProfile call.
type ApplicationSetCharmProfile struct {
	// ApplicationName is the name of the application to set the profile on.
	ApplicationName string `json:"application"`

	// CharmURL is the new charm's url.
	CharmURL string `json:"charm-url"`
}

// ApplicationExpose holds the parameters for making the application Expose call.
type ApplicationExpose struct {
	ApplicationName string `json:"application"`
}

// ApplicationSet holds the parameters for an application Set
// command. Options contains the configuration data.
type ApplicationSet struct {
	ApplicationName string            `json:"application"`
	Options         map[string]string `json:"options"`
}

// ApplicationUnset holds the parameters for an application Unset
// command. Options contains the option attribute names
// to unset.
type ApplicationUnset struct {
	ApplicationName string   `json:"application"`
	Options         []string `json:"options"`
}

// ApplicationGet holds parameters for making the Get or
// GetCharmURL calls.
type ApplicationGet struct {
	ApplicationName string `json:"application"`
}

// ApplicationGetResults holds results of the application Get call.
type ApplicationGetResults struct {
	Application       string                 `json:"application"`
	Charm             string                 `json:"charm"`
	CharmConfig       map[string]interface{} `json:"config"`
	ApplicationConfig map[string]interface{} `json:"application-config,omitempty"`
	Constraints       constraints.Value      `json:"constraints"`
	Series            string                 `json:"series"`
}

// ApplicationConfigSetArgs holds the parameters for
// setting application config values for specified applications.
type ApplicationConfigSetArgs struct {
	Args []ApplicationConfigSet
}

// ApplicationConfigSet holds the parameters for an application
// config set command.
type ApplicationConfigSet struct {
	ApplicationName string            `json:"application"`
	Config          map[string]string `json:"config"`
}

// ApplicationConfigUnsetArgs holds the parameters for
// resetting application config values for specified applications.
type ApplicationConfigUnsetArgs struct {
	Args []ApplicationUnset
}

// ApplicationCharmRelations holds parameters for making the application CharmRelations call.
type ApplicationCharmRelations struct {
	ApplicationName string `json:"application"`
}

// ApplicationCharmRelationsResults holds the results of the application CharmRelations call.
type ApplicationCharmRelationsResults struct {
	CharmRelations []string `json:"charm-relations"`
}

// ApplicationUnexpose holds parameters for the application Unexpose call.
type ApplicationUnexpose struct {
	ApplicationName string `json:"application"`
}

// ApplicationMetricCredential holds parameters for the SetApplicationCredentials call.
type ApplicationMetricCredential struct {
	ApplicationName   string `json:"application"`
	MetricCredentials []byte `json:"metrics-credentials"`
}

// ApplicationMetricCredentials holds multiple ApplicationMetricCredential parameters.
type ApplicationMetricCredentials struct {
	Creds []ApplicationMetricCredential `json:"creds"`
}

// ApplicationGetConfigResults holds the return values for application GetConfig.
type ApplicationGetConfigResults struct {
	Results []ConfigResult
}

// ConfigResults holds configuration values for an entity.
type ConfigResult struct {
	Config map[string]interface{} `json:"config"`
	Error  *Error                 `json:"error,omitempty"`
}

// OperatorProvisioningInfo holds info need to provision an operator.
type OperatorProvisioningInfo struct {
	ImagePath    string                     `json:"image-path"`
	Version      version.Number             `json:"version"`
	APIAddresses []string                   `json:"api-addresses"`
	Tags         map[string]string          `json:"tags,omitempty"`
	CharmStorage KubernetesFilesystemParams `json:"charm-storage"`
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
	Units          []ApplicationUnitParams `json:"units"`
}

// ApplicationUnitParams holds unit parameters used to update a unit.
type ApplicationUnitParams struct {
	ProviderId     string                     `json:"provider-id"`
	UnitTag        string                     `json:"unit-tag"`
	Address        string                     `json:"address"`
	Ports          []string                   `json:"ports"`
	FilesystemInfo []KubernetesFilesystemInfo `json:"filesystem-info,omitempty"`
	Status         string                     `json:"status"`
	Info           string                     `json:"info"`
	Data           map[string]interface{}     `json:"data,omitempty"`
}

// UpdateApplicationServiceArgs holds the parameters for
// updating application services.
type UpdateApplicationServiceArgs struct {
	Args []UpdateApplicationServiceArg `json:"args"`
}

// UpdateApplicationServiceArg holds parameters used to update
// an application's service definition for the cloud.
type UpdateApplicationServiceArg struct {
	ApplicationTag string    `json:"application-tag"`
	ProviderId     string    `json:"provider-id"`
	Addresses      []Address `json:"addresses"`
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
}

// ApplicationDestroy holds the parameters for making the deprecated
// Application.Destroy call.
type ApplicationDestroy struct {
	ApplicationName string `json:"application"`
}

// DestroyApplicationsParams holds bulk parameters for the
// Application.DestroyApplication call.
type DestroyApplicationsParams struct {
	Applications []DestroyApplicationParams `json:"applications"`
}

// DestroyApplicationParams holds parameters for the
// Application.DestroyApplication call.
type DestroyApplicationParams struct {
	// ApplicationTag holds the tag of the application to destroy.
	ApplicationTag string `json:"application-tag"`

	// DestroyStorage controls whether or not storage attached to
	// units of the application should be destroyed.
	DestroyStorage bool `json:"destroy-storage,omitempty"`
}

// DestroyConsumedApplicationsParams holds bulk parameters for the
// Application.DestroyConsumedApplication call.
type DestroyConsumedApplicationsParams struct {
	Applications []DestroyConsumedApplicationParams `json:"applications"`
}

// DestroyConsumedApplicationParams holds the parameters for the
// RemoteApplication.Destroy call.
type DestroyConsumedApplicationParams struct {
	ApplicationTag string `json:"application-tag"`
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
	AuthTag     string           `json:"auth-tag"`
	Credentials string           `json:"credentials"`
	Nonce       string           `json:"nonce"`
	Macaroons   []macaroon.Slice `json:"macaroons"`
	CLIArgs     string           `json:"cli-args,omitempty"`
	UserData    string           `json:"user-data"`
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

// GetApplicationConstraints stores parameters for making the GetApplicationConstraints call.
type GetApplicationConstraints struct {
	ApplicationName string `json:"application"`
}

// GetConstraintsResults holds results of the GetConstraints call.
type GetConstraintsResults struct {
	Constraints constraints.Value `json:"constraints"`
}

// ApplicationGetConstraintsResults holds the multiple return values for GetConstraints call.
type ApplicationGetConstraintsResults struct {
	Results []ApplicationConstraint `json:"results"`
}

// ApplicationConstraint holds the constraints value for a single application, or
// an error for trying to get it.
type ApplicationConstraint struct {
	Constraints constraints.Value `json:"constraints"`
	Error       *Error            `json:"error,omitempty"`
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

// AllWatcherId holds the id of an AllWatcher.
type AllWatcherId struct {
	AllWatcherId string `json:"watcher-id"`
}

// AllWatcherNextResults holds deltas returned from calling AllWatcher.Next().
type AllWatcherNextResults struct {
	Deltas []multiwatcher.Delta `json:"deltas"`
}

// ListSSHKeys stores parameters used for a KeyManager.ListKeys call.
type ListSSHKeys struct {
	Entities `json:"entities"`
	Mode     ssh.ListMode `json:"mode"`
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
	SharedSecret   string `json:"shared-secret"`
	SystemIdentity string `json:"system-identity"`
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
	AptMirror                  string                 `json:"apt-mirror"`
	CloudInitUserData          map[string]interface{} `json:"cloudinit-userdata,omitempty"`
	ContainerInheritProperties string                 `json:"container-inherit-properties,omitempty"`
	*UpdateBehavior
}

// ContainerConfigV5 contains information from the model config that is
// needed for container cloud-init for version 5 provisioner api calls.
type ContainerConfigV5 struct {
	ProviderType               string                 `json:"provider-type"`
	AuthorizedKeys             string                 `json:"authorized-keys"`
	SSLHostnameVerification    bool                   `json:"ssl-hostname-verification"`
	Proxy                      proxy.Settings         `json:"proxy"`
	AptProxy                   proxy.Settings         `json:"apt-proxy"`
	AptMirror                  string                 `json:"apt-mirror"`
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

// JobsResult holds the jobs for a machine that are returned by a call to Jobs.
type JobsResult struct {
	Jobs  []multiwatcher.MachineJob `json:"jobs"`
	Error *Error                    `json:"error,omitempty"`
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

// ControllersServersSpec contains arguments for
// the EnableHA client API call.
type ControllersSpec struct {
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
	Specs []ControllersSpec `json:"specs"`
}

// ControllersChangeResult contains the results
// of a single EnableHA API call or
// an error.
type ControllersChangeResult struct {
	Result ControllersChanges `json:"result"`
	Error  *Error             `json:"error,omitempty"`
}

// ControllersChangeResults contains the results
// of the EnableHA API call.
type ControllersChangeResults struct {
	Results []ControllersChangeResult `json:"results"`
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
	Number version.Number `json:"number"`

	// MajorVersion will be used to match the major version if non-zero.
	MajorVersion int `json:"major"`

	// MinorVersion will be used to match the major version if greater
	// than or equal to zero, and Number is zero.
	MinorVersion int `json:"minor"`

	// Arch will be used to match tools by architecture if non-empty.
	Arch string `json:"arch"`

	// Series will be used to match tools by series if non-empty.
	Series string `json:"series"`

	// AgentStream will be used to set agent stream to search
	AgentStream string `json:"agentstream"`
}

// FindToolsResult holds a list of tools from FindTools and any error.
type FindToolsResult struct {
	List  tools.List `json:"list"`
	Error *Error     `json:"error,omitempty"`
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
	Time     time.Time `json:"t"`
	Module   string    `json:"m"`
	Location string    `json:"l"`
	Level    string    `json:"v"`
	Message  string    `json:"x"`
	Entity   string    `json:"e,omitempty"`
}

// PubSubMessage is used to propagate pubsub messages from one api server to the
// others.
type PubSubMessage struct {
	Topic string                 `json:"topic"`
	Data  map[string]interface{} `json:"data"`
}

// BundleChangesParams holds parameters for making Bundle.GetChanges calls.
type BundleChangesParams struct {
	// BundleDataYAML is the YAML-encoded charm bundle data
	// (see "github.com/juju/charm.BundleData").
	BundleDataYAML string `json:"yaml"`
	BundleURL      string `json:"bundleURL"`
}

// BundleChangesResults holds results of the Bundle.GetChanges call.
type BundleChangesResults struct {
	// Changes holds the list of changes required to deploy the bundle.
	// It is omitted if the provided bundle YAML has verification errors.
	Changes []*BundleChange `json:"changes,omitempty"`
	// Errors holds possible bundle verification errors.
	Errors []string `json:"errors,omitempty"`
}

// BundleChange holds a single change required to deploy a bundle.
type BundleChange struct {
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

type MongoVersion struct {
	Major         int    `json:"major"`
	Minor         int    `json:"minor"`
	Patch         string `json:"patch"`
	StorageEngine string `json:"engine"`
}

// UpgradeMongoParams holds the arguments required to
// enter upgrade mongo mode.
type UpgradeMongoParams struct {
	Target MongoVersion `json:"target"`
}

// HAMember holds information that identifies one member
// of HA.
type HAMember struct {
	Tag           string          `json:"tag"`
	PublicAddress network.Address `json:"public-address"`
	Series        string          `json:"series"`
}

// MongoUpgradeResults holds the results of an attempt
// to enter upgrade mongo mode.
type MongoUpgradeResults struct {
	RsMembers []replicaset.Member `json:"rs-members"`
	Master    HAMember            `json:"master"`
	Members   []HAMember          `json:"ha-members"`
}

// ResumeReplicationParams holds the members of a HA that
// must be resumed.
type ResumeReplicationParams struct {
	Members []replicaset.Member `json:"members"`
}

// MeterStatusParam holds meter status information to be set for the specified tag.
type MeterStatusParam struct {
	Tag  string `json:"tag"`
	Code string `json:"code"`
	Info string `json:"info,omitempty"`
}

// MeterStatusParams holds parameters for making SetMeterStatus calls.
type MeterStatusParams struct {
	Statuses []MeterStatusParam `json:"statues"`
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
	// DetachedStorage is the tags of storage instances that will be
	// detached from the machine (assigned units) as a result of
	// destroying the machine, and will remain in the model after
	// the machine and unit are removed.
	DetachedStorage []Entity `json:"detached-storage,omitempty"`

	// DestroyedStorage is the tags of storage instances that will be
	// destroyed as a result of destroying the machine.
	DestroyedStorage []Entity `json:"destroyed-storage,omitempty"`

	// DestroyedStorage is the tags of units that will be destroyed
	// as a result of destroying the machine.
	DestroyedUnits []Entity `json:"destroyed-units,omitempty"`
}

// DestroyApplicationResults contains the results of a DestroyApplication
// API request.
type DestroyApplicationResults struct {
	Results []DestroyApplicationResult `json:"results,omitempty"`
}

// DestroyApplicationResult contains one of the results of a
// DestroyApplication API request.
type DestroyApplicationResult struct {
	Error *Error                  `json:"error,omitempty"`
	Info  *DestroyApplicationInfo `json:"info,omitempty"`
}

// DestroyApplicationInfo contains information related to the removal of
// an application.
type DestroyApplicationInfo struct {
	// DetachedStorage is the tags of storage instances that will be
	// detached from the application's units, and will remain in the
	// model after the units are removed.
	DetachedStorage []Entity `json:"detached-storage,omitempty"`

	// DestroyedStorage is the tags of storage instances that will be
	// destroyed as a result of destroying the application.
	DestroyedStorage []Entity `json:"destroyed-storage,omitempty"`

	// DestroyedUnits is the tags of units that will be destroyed
	// as a result of destroying the application.
	DestroyedUnits []Entity `json:"destroyed-units,omitempty"`
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

// ScaleApplicationsParams holds bulk parameters for the Application.ScaleApplication call.
type ScaleApplicationsParams struct {
	Applications []ScaleApplicationParams `json:"applications"`
}

// ScaleApplicationParams holds parameters for the Application.ScaleApplication call.
type ScaleApplicationParams struct {
	// ApplicationTag holds the tag of the application to scale.
	ApplicationTag string `json:"application-tag"`

	// Scale is the number of units which should be running.
	Scale int `json:"scale"`
}

// ScaleApplicationResults contains the results of a ScaleApplication
// API request.
type ScaleApplicationResults struct {
	Results []ScaleApplicationResult `json:"results,omitempty"`
}

// ScaleApplicationResult contains one of the results of a
// ScaleApplication API request.
type ScaleApplicationResult struct {
	Error *Error                `json:"error,omitempty"`
	Info  *ScaleApplicationInfo `json:"info,omitempty"`
}

// ScaleApplicationInfo contains information related to the scaling of
// an application.
type ScaleApplicationInfo struct {
	// Scale is the number of units which should be running.
	Scale int `json:"num-units"`
}

// DumpModelRequest wraps the request for a dump-model call.
// A simplified dump will not contain a complete export, but instead
// a reduced set that is determined by the server.
type DumpModelRequest struct {
	Entities   []Entity `json:"entities"`
	Simplified bool     `json:"simplified"`
}

// UpgradeSeriesStatusResult contains the upgrade series status result for an upgrading
// machine or unit
type UpgradeSeriesStatusResult struct {
	Error  *Error                    `json:"error,omitempty"`
	Status model.UpgradeSeriesStatus `json:"status,omitempty"`
}

// UpgradeSeriesStatusResults contains the upgrade series status results for
// upgrading machines or units.
type UpgradeSeriesStatusResults struct {
	Results []UpgradeSeriesStatusResult `json:"results,omitempty"`
}

// UpgradeSeriesStatusParams contains the entities and desired statuses for
// those entities.
type UpgradeSeriesStatusParams struct {
	Params []UpgradeSeriesStatusParam `json:"params"`
}

// UpgradeSeriesStatusParam contains the entity and desired status for that
// entity along with a context message describing why the change to the status
// is being requested.
type UpgradeSeriesStatusParam struct {
	Entity  Entity                    `json:"entity"`
	Status  model.UpgradeSeriesStatus `json:"status"`
	Message string                    `json:"message"`
}

// UpgradeSeriesStartUnitCompletionParam contains entities and a context message.
type UpgradeSeriesStartUnitCompletionParam struct {
	Entities []Entity `json:"entities"`
	Message  string   `json:"message"`
}

type UpgradeSeriesNotificationParams struct {
	Params []UpgradeSeriesNotificationParam `json:"params"`
}

type UpgradeSeriesNotificationParam struct {
	Entity    Entity `json:"entity"`
	WatcherId string `json:"watcher-id"`
}

// UpgradeSeriesUnitsResults contains the units affected by a series per
// machine entity.
type UpgradeSeriesUnitsResults struct {
	Results []UpgradeSeriesUnitsResult
}

// UpgradeSeriesUnitsResults contains the units affected by a series for
// a given machine.
type UpgradeSeriesUnitsResult struct {
	Error     *Error   `json:"error,omitempty"`
	UnitNames []string `json:"unit-names"`
}
