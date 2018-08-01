// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"encoding/base64"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

// Application represents a deployed charm in a model.
type Application interface {
	HasAnnotations
	HasConstraints
	HasStatus
	HasStatusHistory

	Tag() names.ApplicationTag
	Name() string
	Type() string
	Series() string
	Subordinate() bool
	CharmURL() string
	Channel() string
	CharmModifiedVersion() int
	ForceCharm() bool
	Exposed() bool
	MinUnits() int

	PasswordHash() string
	PodSpec() string
	CloudService() CloudService
	SetCloudService(CloudServiceArgs)

	EndpointBindings() map[string]string

	CharmConfig() map[string]interface{}
	ApplicationConfig() map[string]interface{}

	Leader() string
	LeadershipSettings() map[string]interface{}

	MetricsCredentials() []byte
	StorageConstraints() map[string]StorageConstraint

	Resources() []Resource
	AddResource(ResourceArgs) Resource

	Units() []Unit
	AddUnit(UnitArgs) Unit

	Tools() AgentTools
	SetTools(AgentToolsArgs)

	Validate() error
}

type applications struct {
	Version       int            `yaml:"version"`
	Applications_ []*application `yaml:"applications"`
}

type application struct {
	Name_                 string `yaml:"name"`
	Type_                 string `yaml:"type"`
	Series_               string `yaml:"series"`
	Subordinate_          bool   `yaml:"subordinate,omitempty"`
	CharmURL_             string `yaml:"charm-url"`
	Channel_              string `yaml:"cs-channel"`
	CharmModifiedVersion_ int    `yaml:"charm-mod-version"`

	// ForceCharm is true if an upgrade charm is forced.
	// It means upgrade even if the charm is in an error state.
	ForceCharm_ bool `yaml:"force-charm,omitempty"`
	Exposed_    bool `yaml:"exposed,omitempty"`
	MinUnits_   int  `yaml:"min-units,omitempty"`

	Status_        *status `yaml:"status"`
	StatusHistory_ `yaml:"status-history"`

	EndpointBindings_ map[string]string `yaml:"endpoint-bindings,omitempty"`

	CharmConfig_       map[string]interface{} `yaml:"settings"`
	ApplicationConfig_ map[string]interface{} `yaml:"application-config,omitempty"`

	Leader_             string                 `yaml:"leader,omitempty"`
	LeadershipSettings_ map[string]interface{} `yaml:"leadership-settings"`

	MetricsCredentials_ string `yaml:"metrics-creds,omitempty"`

	// unit count will be assumed by the number of units associated.
	Units_ units `yaml:"units"`

	Resources_ resources `yaml:"resources"`

	Annotations_ `yaml:"annotations,omitempty"`

	Constraints_        *constraints                  `yaml:"constraints,omitempty"`
	StorageConstraints_ map[string]*storageconstraint `yaml:"storage-constraints,omitempty"`

	// CAAS application fields.
	PasswordHash_ string        `yaml:"password-hash,omitempty"`
	PodSpec_      string        `yaml:"pod-spec,omitempty"`
	CloudService_ *cloudService `yaml:"cloud-service,omitempty"`
	Tools_        *agentTools   `yaml:"tools,omitempty"`
}

// ApplicationArgs is an argument struct used to add an application to the Model.
type ApplicationArgs struct {
	Tag                  names.ApplicationTag
	Type                 string
	Series               string
	Subordinate          bool
	CharmURL             string
	Channel              string
	CharmModifiedVersion int
	ForceCharm           bool
	PasswordHash         string
	PodSpec              string
	CloudService         *CloudServiceArgs
	Exposed              bool
	MinUnits             int
	EndpointBindings     map[string]string
	ApplicationConfig    map[string]interface{}
	CharmConfig          map[string]interface{}
	Leader               string
	LeadershipSettings   map[string]interface{}
	StorageConstraints   map[string]StorageConstraintArgs
	MetricsCredentials   []byte
}

func newApplication(args ApplicationArgs) *application {
	creds := base64.StdEncoding.EncodeToString(args.MetricsCredentials)
	app := &application{
		Name_:                 args.Tag.Id(),
		Type_:                 args.Type,
		Series_:               args.Series,
		Subordinate_:          args.Subordinate,
		CharmURL_:             args.CharmURL,
		Channel_:              args.Channel,
		CharmModifiedVersion_: args.CharmModifiedVersion,
		ForceCharm_:           args.ForceCharm,
		Exposed_:              args.Exposed,
		PasswordHash_:         args.PasswordHash,
		PodSpec_:              args.PodSpec,
		CloudService_:         newCloudService(args.CloudService),
		MinUnits_:             args.MinUnits,
		EndpointBindings_:     args.EndpointBindings,
		ApplicationConfig_:    args.ApplicationConfig,
		CharmConfig_:          args.CharmConfig,
		Leader_:               args.Leader,
		LeadershipSettings_:   args.LeadershipSettings,
		MetricsCredentials_:   creds,
		StatusHistory_:        newStatusHistory(),
	}
	app.setUnits(nil)
	app.setResources(nil)
	if len(args.StorageConstraints) > 0 {
		app.StorageConstraints_ = make(map[string]*storageconstraint)
		for key, value := range args.StorageConstraints {
			app.StorageConstraints_[key] = newStorageConstraint(value)
		}
	}
	return app
}

// Tag implements Application.
func (a *application) Tag() names.ApplicationTag {
	return names.NewApplicationTag(a.Name_)
}

// Name implements Application.
func (a *application) Name() string {
	return a.Name_
}

// Type implements Application
func (a *application) Type() string {
	return a.Type_
}

// Series implements Application.
func (a *application) Series() string {
	return a.Series_
}

// Subordinate implements Application.
func (a *application) Subordinate() bool {
	return a.Subordinate_
}

// CharmURL implements Application.
func (a *application) CharmURL() string {
	return a.CharmURL_
}

// Channel implements Application.
func (a *application) Channel() string {
	return a.Channel_
}

// CharmModifiedVersion implements Application.
func (a *application) CharmModifiedVersion() int {
	return a.CharmModifiedVersion_
}

// ForceCharm implements Application.
func (a *application) ForceCharm() bool {
	return a.ForceCharm_
}

// Exposed implements Application.
func (a *application) Exposed() bool {
	return a.Exposed_
}

// PasswordHash implements Application.
func (a *application) PasswordHash() string {
	return a.PasswordHash_
}

// PodSpec implements Application.
func (a *application) PodSpec() string {
	return a.PodSpec_
}

// MinUnits implements Application.
func (a *application) MinUnits() int {
	return a.MinUnits_
}

// EndpointBindings implements Application.
func (a *application) EndpointBindings() map[string]string {
	return a.EndpointBindings_
}

// ApplicationConfig implements Application.
func (a *application) ApplicationConfig() map[string]interface{} {
	return a.ApplicationConfig_
}

// CharmConfig implements Application.
func (a *application) CharmConfig() map[string]interface{} {
	return a.CharmConfig_
}

// Leader implements Application.
func (a *application) Leader() string {
	return a.Leader_
}

// LeadershipSettings implements Application.
func (a *application) LeadershipSettings() map[string]interface{} {
	return a.LeadershipSettings_
}

// StorageConstraints implements Application.
func (a *application) StorageConstraints() map[string]StorageConstraint {
	result := make(map[string]StorageConstraint)
	for key, value := range a.StorageConstraints_ {
		result[key] = value
	}
	return result
}

// MetricsCredentials implements Application.
func (a *application) MetricsCredentials() []byte {
	// Here we are explicitly throwing away any decode error. We check that
	// the creds can be decoded when we parse the incoming data, or we encode
	// an incoming byte array, so in both cases, we know that the stored creds
	// can be decoded.
	creds, _ := base64.StdEncoding.DecodeString(a.MetricsCredentials_)
	return creds
}

// Status implements Application.
func (a *application) Status() Status {
	// To avoid typed nils check nil here.
	if a.Status_ == nil {
		return nil
	}
	return a.Status_
}

// SetStatus implements Application.
func (a *application) SetStatus(args StatusArgs) {
	a.Status_ = newStatus(args)
}

// Units implements Application.
func (a *application) Units() []Unit {
	result := make([]Unit, len(a.Units_.Units_))
	for i, u := range a.Units_.Units_ {
		result[i] = u
	}
	return result
}

func (a *application) unitNames() set.Strings {
	result := set.NewStrings()
	for _, u := range a.Units_.Units_ {
		result.Add(u.Name())
	}
	return result
}

// AddUnit implements Application.
func (a *application) AddUnit(args UnitArgs) Unit {
	u := newUnit(args)
	a.Units_.Units_ = append(a.Units_.Units_, u)
	return u
}

func (a *application) setUnits(unitList []*unit) {
	a.Units_ = units{
		Version: 2,
		Units_:  unitList,
	}
}

// Constraints implements HasConstraints.
func (a *application) Constraints() Constraints {
	if a.Constraints_ == nil {
		return nil
	}
	return a.Constraints_
}

// SetConstraints implements HasConstraints.
func (a *application) SetConstraints(args ConstraintsArgs) {
	a.Constraints_ = newConstraints(args)
}

// CloudService implements Application.
func (a *application) CloudService() CloudService {
	if a.CloudService_ == nil {
		return nil
	}
	return a.CloudService_
}

// SetCloudService implements Application.
func (a *application) SetCloudService(args CloudServiceArgs) {
	a.CloudService_ = newCloudService(&args)
}

// Resources implements Application.
func (a *application) Resources() []Resource {
	rs := a.Resources_.Resources_
	result := make([]Resource, len(rs))
	for i, r := range rs {
		result[i] = r
	}
	return result
}

// AddResource implements Application.
func (a *application) AddResource(args ResourceArgs) Resource {
	r := newResource(args)
	a.Resources_.Resources_ = append(a.Resources_.Resources_, r)
	return r
}

func (a *application) setResources(resourceList []*resource) {
	a.Resources_ = resources{
		Version:    1,
		Resources_: resourceList,
	}
}

// Tools implements Application.
func (a *application) Tools() AgentTools {
	// To avoid a typed nil, check before returning.
	if a.Tools_ == nil {
		return nil
	}
	return a.Tools_
}

// SetTools implements Application.
func (a *application) SetTools(args AgentToolsArgs) {
	a.Tools_ = newAgentTools(args)
}

// Validate implements Application.
func (a *application) Validate() error {
	if a.Name_ == "" {
		return errors.NotValidf("application missing name")
	}
	if a.Status_ == nil {
		return errors.NotValidf("application %q missing status", a.Name_)
	}

	if a.Tools_ == nil && a.Type_ == CAAS {
		return errors.NotValidf("application %q missing tools", a.Name_)
	}

	for _, resource := range a.Resources_.Resources_ {
		if err := resource.Validate(); err != nil {
			return errors.Annotatef(err, "resource %s", resource.Name_)
		}
	}

	// If leader is set, it must match one of the units.
	var leaderFound bool
	// All of the applications units should also be valid.
	for _, u := range a.Units() {
		if err := u.Validate(); err != nil {
			return errors.Trace(err)
		}
		// We know that the unit has a name, because it validated correctly.
		if u.Name() == a.Leader_ {
			leaderFound = true
		}
	}
	if a.Leader_ != "" && !leaderFound {
		return errors.NotValidf("missing unit for leader %q", a.Leader_)
	}
	return nil
}

func importApplications(source map[string]interface{}) ([]*application, error) {
	checker := versionedChecker("applications")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "applications version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := applicationDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["applications"].([]interface{})
	return importApplicationList(sourceList, importFunc)
}

func importApplicationList(sourceList []interface{}, importFunc applicationDeserializationFunc) ([]*application, error) {
	result := make([]*application, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for application %d, %T", i, value)
		}
		application, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "application %d", i)
		}
		result = append(result, application)
	}
	return result, nil
}

type applicationDeserializationFunc func(map[string]interface{}) (*application, error)

var applicationDeserializationFuncs = map[int]applicationDeserializationFunc{
	1: importApplicationV1,
	2: importApplicationV2,
	3: importApplicationV3,
}

func applicationV1Fields() (schema.Fields, schema.Defaults) {
	fields := schema.Fields{
		"name":                schema.String(),
		"series":              schema.String(),
		"subordinate":         schema.Bool(),
		"charm-url":           schema.String(),
		"cs-channel":          schema.String(),
		"charm-mod-version":   schema.Int(),
		"force-charm":         schema.Bool(),
		"exposed":             schema.Bool(),
		"min-units":           schema.Int(),
		"status":              schema.StringMap(schema.Any()),
		"endpoint-bindings":   schema.StringMap(schema.String()),
		"settings":            schema.StringMap(schema.Any()),
		"leader":              schema.String(),
		"leadership-settings": schema.StringMap(schema.Any()),
		"storage-constraints": schema.StringMap(schema.StringMap(schema.Any())),
		"metrics-creds":       schema.String(),
		"resources":           schema.StringMap(schema.Any()),
		"units":               schema.StringMap(schema.Any()),
	}

	defaults := schema.Defaults{
		"subordinate":         false,
		"force-charm":         false,
		"exposed":             false,
		"min-units":           int64(0),
		"leader":              "",
		"metrics-creds":       "",
		"storage-constraints": schema.Omit,
		"endpoint-bindings":   schema.Omit,
		"application-config":  schema.Omit,
	}
	addAnnotationSchema(fields, defaults)
	addConstraintsSchema(fields, defaults)
	addStatusHistorySchema(fields)
	return fields, defaults
}

func applicationV2Fields() (schema.Fields, schema.Defaults) {
	fields, defaults := applicationV1Fields()
	fields["type"] = schema.String()
	return fields, defaults
}

func applicationV3Fields() (schema.Fields, schema.Defaults) {
	fields, defaults := applicationV2Fields()
	fields["application-config"] = schema.StringMap(schema.Any())
	fields["password-hash"] = schema.String()
	fields["pod-spec"] = schema.String()
	fields["cloud-service"] = schema.StringMap(schema.Any())
	fields["tools"] = schema.StringMap(schema.Any())
	defaults["password-hash"] = ""
	defaults["pod-spec"] = ""
	defaults["cloud-service"] = schema.Omit
	defaults["tools"] = schema.Omit
	return fields, defaults
}

func importApplicationV1(source map[string]interface{}) (*application, error) {
	fields, defaults := applicationV1Fields()
	return importApplication(fields, defaults, 1, source)
}

func importApplicationV2(source map[string]interface{}) (*application, error) {
	fields, defaults := applicationV2Fields()
	return importApplication(fields, defaults, 2, source)
}

func importApplicationV3(source map[string]interface{}) (*application, error) {
	fields, defaults := applicationV3Fields()
	return importApplication(fields, defaults, 3, source)
}

func importApplication(fields schema.Fields, defaults schema.Defaults, importVersion int, source map[string]interface{}) (*application, error) {
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "application schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &application{
		Name_:                 valid["name"].(string),
		Series_:               valid["series"].(string),
		Type_:                 IAAS,
		Subordinate_:          valid["subordinate"].(bool),
		CharmURL_:             valid["charm-url"].(string),
		Channel_:              valid["cs-channel"].(string),
		CharmModifiedVersion_: int(valid["charm-mod-version"].(int64)),
		ForceCharm_:           valid["force-charm"].(bool),
		Exposed_:              valid["exposed"].(bool),
		MinUnits_:             int(valid["min-units"].(int64)),
		EndpointBindings_:     convertToStringMap(valid["endpoint-bindings"]),
		CharmConfig_:          valid["settings"].(map[string]interface{}),
		Leader_:               valid["leader"].(string),
		LeadershipSettings_:   valid["leadership-settings"].(map[string]interface{}),
		StatusHistory_:        newStatusHistory(),
	}

	if importVersion >= 2 {
		result.Type_ = valid["type"].(string)
	}
	if importVersion >= 3 {
		result.PasswordHash_ = valid["password-hash"].(string)
		result.PodSpec_ = valid["pod-spec"].(string)
	}

	result.importAnnotations(valid)

	if err := result.importStatusHistory(valid); err != nil {
		return nil, errors.Trace(err)
	}

	if configValues, ok := valid["application-config"]; ok {
		configMap, ok := configValues.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for application-config, %T", configValues)
		}
		result.ApplicationConfig_ = configMap
	}

	if constraintsMap, ok := valid["constraints"]; ok {
		constraints, err := importConstraints(constraintsMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Constraints_ = constraints
	}

	if constraintsMap, ok := valid["storage-constraints"]; ok {
		constraints, err := importStorageConstraints(constraintsMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.StorageConstraints_ = constraints
	}

	if cloudServiceMap, ok := valid["cloud-service"]; ok {
		cloudService, err := importCloudService(cloudServiceMap.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.CloudService_ = cloudService
	}

	toolsMap, ok := valid["tools"].(map[string]interface{})
	// CAAS models require tools.
	if importVersion >= 3 && !ok && result.Type_ == CAAS {
		return nil, errors.NotFoundf("tools metadata in CAAS model")
	}
	if ok {
		tools, err := importAgentTools(toolsMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Tools_ = tools
	}

	encodedCreds := valid["metrics-creds"].(string)
	// The model stores the creds encoded, but we want to make sure that
	// we are storing something that can be decoded.
	if _, err := base64.StdEncoding.DecodeString(encodedCreds); err != nil {
		return nil, errors.Annotate(err, "metrics credentials not valid")
	}
	result.MetricsCredentials_ = encodedCreds

	status, err := importStatus(valid["status"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.Status_ = status

	resources, err := importResources(valid["resources"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.setResources(resources)

	units, err := importUnits(valid["units"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Units inherit model type from their application.
	for _, u := range units {
		u.Type_ = result.Type_

		// Validate to ensure expected type specific
		// attributes like tools are set.
		if err := u.Validate(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	result.setUnits(units)

	return result, nil
}
