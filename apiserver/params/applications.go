// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/storage"
)

// ApplicationsDeploy holds the parameters for deploying one or more applications.
type ApplicationsDeploy struct {
	Applications []ApplicationDeploy `json:"applications"`
}

// CharmOrigin holds the parameters for the optional location of the source of
// the charm.
type CharmOrigin struct {
	Source string `json:"source"`
}

// ApplicationDeploy holds the parameters for making the application Deploy
// call.
type ApplicationDeploy struct {
	ApplicationName  string                         `json:"application"`
	Series           string                         `json:"series"`
	CharmURL         string                         `json:"charm-url"`
	CharmOrigin      *CharmOrigin                   `json:"charm-origin,omitempty"`
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

// ApplicationsDeployV12 holds the parameters for deploying one or more
// applications.
type ApplicationsDeployV12 struct {
	Applications []ApplicationDeployV12 `json:"applications"`
}

// ApplicationDeployV12 holds the parameters for making the application Deploy
// call for application facades older than v12.
// Missing the newer CharmOrigin.
type ApplicationDeployV12 struct {
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

// ApplicationsDeployV5 holds the parameters for deploying one or more
// applications.
type ApplicationsDeployV5 struct {
	Applications []ApplicationDeployV5 `json:"applications"`
}

// ApplicationDeployV5 holds the parameters for making the application Deploy
// call for application facades older than v6. Missing the newer Policy arg.
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

// ApplicationDeployV6 holds the parameters for making the application Deploy
// call for application facades older than v6. Missing the newer Device arg.
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
	CharmOrigin     *CharmOrigin       `json:"charm-origin,omitempty"`
	ForceCharmURL   bool               `json:"force-charm-url"`
	ForceSeries     bool               `json:"force-series"`
	Force           bool               `json:"force"`
	MinUnits        *int               `json:"min-units,omitempty"`
	SettingsStrings map[string]string  `json:"settings,omitempty"`
	SettingsYAML    string             `json:"settings-yaml"` // Takes precedence over SettingsStrings if both are present.
	Constraints     *constraints.Value `json:"constraints,omitempty"`

	// Generation is the generation version in which this
	// request will update the application.
	Generation string `json:"generation"`
}

// ApplicationSetCharm sets the charm for a given application.
type ApplicationSetCharm struct {
	// ApplicationName is the name of the application to set the charm on.
	ApplicationName string `json:"application"`

	// Generation is the generation version that this
	// request will set the application charm for.
	Generation string `json:"generation"`

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

	// Force forces the lxd profile validation overriding even if it's fails.
	Force bool `json:"force"`

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

	// EndpointBindings is a map of operator-defined endpoint names to
	// space names to be merged with any existing endpoint bindings. This
	// field is only understood by Application facade version 10 and greater.
	EndpointBindings map[string]string `json:"endpoint-bindings,omitempty"`
}

// ApplicationExpose holds the parameters for making the application Expose call.
type ApplicationExpose struct {
	ApplicationName string `json:"application"`
}

// ApplicationSet holds the parameters for an application Set
// command. Options contains the configuration data.
type ApplicationSet struct {
	ApplicationName string `json:"application"`

	// BranchName identifies the "in-flight" branch that this
	// request will set configuration for.
	BranchName string `json:"branch"`

	Options map[string]string `json:"options"`
}

// ApplicationUnset holds the parameters for an application Unset
// command. Options contains the option attribute names
// to unset.
type ApplicationUnset struct {
	ApplicationName string `json:"application"`

	// BranchName identifies the "in-flight" branch that this
	// request will unset configuration for.
	BranchName string `json:"branch"`

	Options []string `json:"options"`
}

// ApplicationGetArgs is used to request config for
// multiple application/generation pairs.
type ApplicationGetArgs struct {
	Args []ApplicationGet `json:"args"`
}

// ApplicationGet holds parameters for making the singular Get or GetCharmURL
// calls, and bulk calls to CharmConfig in the V9 API.
type ApplicationGet struct {
	ApplicationName string `json:"application"`

	// BranchName identifies the "in-flight" branch that this
	// request will retrieve application data for.
	BranchName string `json:"branch"`
}

// ApplicationGetResults holds results of the application Get call.
type ApplicationGetResults struct {
	Application       string                 `json:"application"`
	Charm             string                 `json:"charm"`
	CharmConfig       map[string]interface{} `json:"config"`
	ApplicationConfig map[string]interface{} `json:"application-config,omitempty"`
	Constraints       constraints.Value      `json:"constraints"`
	Series            string                 `json:"series"`
	Channel           string                 `json:"channel"`
	EndpointBindings  map[string]string      `json:"endpoint-bindings,omitempty"`
}

// ApplicationConfigSetArgs holds the parameters for
// setting application config values for specified applications.
type ApplicationConfigSetArgs struct {
	Args []ApplicationConfigSet
}

// ApplicationConfigSet holds the parameters for an application
// config set command.
type ApplicationConfigSet struct {
	ApplicationName string `json:"application"`

	// Generation is the generation version that this request
	// will set application configuration for.
	Generation string `json:"generation"`

	Config map[string]string `json:"config"`
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

	Scale      *int   `json:"scale,omitempty"`
	Generation *int64 `json:"generation,omitempty"`
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

	// Force controls whether or not the destruction of an application
	// will be forced, i.e. ignore operational errors.
	Force bool `json:"force"`

	// MaxWait specifies the amount of time that each step in application removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
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

	// Force controls whether or not the destruction process ignores
	// operational errors. When true, the process will ignore them.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each step in application removal
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`
}

// GetApplicationConstraints stores parameters for making the GetApplicationConstraints call.
type GetApplicationConstraints struct {
	ApplicationName string `json:"application"`
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

	// Scale is the number of units which should be added/removed from the existing count.
	ScaleChange int `json:"scale-change,omitempty"`

	// Force controls whether or not scaling of an application
	// will be forced, i.e. ignore operational errors.
	Force bool `json:"force"`
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

// ApplicationResult holds an application info.
// NOTE: we should look to combine ApplicationResult and ApplicationInfo.
type ApplicationResult struct {
	Tag              string            `json:"tag"`
	Charm            string            `json:"charm,omitempty"`
	Series           string            `json:"series,omitempty"`
	Channel          string            `json:"channel,omitempty"`
	Constraints      constraints.Value `json:"constraints,omitempty"`
	Principal        bool              `json:"principal"`
	Exposed          bool              `json:"exposed"`
	Remote           bool              `json:"remote"`
	EndpointBindings map[string]string `json:"endpoint-bindings,omitempty"`
}

// ApplicationInfoResults holds an application info result or a retrieval error.
type ApplicationInfoResult struct {
	Result *ApplicationResult `json:"result,omitempty"`
	Error  *Error             `json:"error,omitempty"`
}

// ApplicationInfoResults holds applications associated with entities.
type ApplicationInfoResults struct {
	Results []ApplicationInfoResult `json:"results"`
}

// RelationData holds information about a unit's relation.
type RelationData struct {
	InScope  bool                   `yaml:"in-scope"`
	UnitData map[string]interface{} `yaml:"data"`
}

// EndpointRelationData holds information about a relation to a given endpoint.
type EndpointRelationData struct {
	Endpoint         string                  `json:"endpoint"`
	CrossModel       bool                    `json:"cross-model"`
	RelatedEndpoint  string                  `json:"related-endpoint"`
	ApplicationData  map[string]interface{}  `yaml:"application-relation-data"`
	UnitRelationData map[string]RelationData `json:"unit-relation-data"`
}

// UnitResult holds unit info.
type UnitResult struct {
	Tag             string                 `json:"tag"`
	WorkloadVersion string                 `json:"workload-version"`
	Machine         string                 `json:"machine,omitempty"`
	OpenedPorts     []string               `json:"opened-ports"`
	PublicAddress   string                 `json:"public-address,omitempty"`
	Charm           string                 `json:"charm"`
	Leader          bool                   `json:"leader,omitempty"`
	RelationData    []EndpointRelationData `json:"relation-data,omitempty"`

	// The following are for CAAS models.
	ProviderId string `json:"provider-id,omitempty"`
	Address    string `json:"address,omitempty"`
}

// UnitInfoResults holds an unit info result or a retrieval error.
type UnitInfoResult struct {
	Result *UnitResult `json:"result,omitempty"`
	Error  *Error      `json:"error,omitempty"`
}

// UnitInfoResults holds units associated with entities.
type UnitInfoResults struct {
	Results []UnitInfoResult `json:"results"`
}
