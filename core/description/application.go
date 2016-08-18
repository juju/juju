// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"encoding/base64"

	"github.com/juju/utils/set"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

type applications struct {
	Version       int            `yaml:"version"`
	Applications_ []*application `yaml:"applications"`
}

type application struct {
	Name_                 string `yaml:"name"`
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

	Settings_ map[string]interface{} `yaml:"settings"`

	Leader_             string                 `yaml:"leader,omitempty"`
	LeadershipSettings_ map[string]interface{} `yaml:"leadership-settings"`

	MetricsCredentials_ string `yaml:"metrics-creds,omitempty"`

	// unit count will be assumed by the number of units associated.
	Units_ units `yaml:"units"`

	Annotations_ `yaml:"annotations,omitempty"`

	Constraints_        *constraints                  `yaml:"constraints,omitempty"`
	StorageConstraints_ map[string]*storageconstraint `yaml:"storage-constraints,omitempty"`
}

// ApplicationArgs is an argument struct used to add an application to the Model.
type ApplicationArgs struct {
	Tag                  names.ApplicationTag
	Series               string
	Subordinate          bool
	CharmURL             string
	Channel              string
	CharmModifiedVersion int
	ForceCharm           bool
	Exposed              bool
	MinUnits             int
	Settings             map[string]interface{}
	Leader               string
	LeadershipSettings   map[string]interface{}
	StorageConstraints   map[string]StorageConstraintArgs
	MetricsCredentials   []byte
}

func newApplication(args ApplicationArgs) *application {
	creds := base64.StdEncoding.EncodeToString(args.MetricsCredentials)
	app := &application{
		Name_:                 args.Tag.Id(),
		Series_:               args.Series,
		Subordinate_:          args.Subordinate,
		CharmURL_:             args.CharmURL,
		Channel_:              args.Channel,
		CharmModifiedVersion_: args.CharmModifiedVersion,
		ForceCharm_:           args.ForceCharm,
		Exposed_:              args.Exposed,
		MinUnits_:             args.MinUnits,
		Settings_:             args.Settings,
		Leader_:               args.Leader,
		LeadershipSettings_:   args.LeadershipSettings,
		MetricsCredentials_:   creds,
		StatusHistory_:        newStatusHistory(),
	}
	app.setUnits(nil)
	if len(args.StorageConstraints) > 0 {
		app.StorageConstraints_ = make(map[string]*storageconstraint)
		for key, value := range args.StorageConstraints {
			app.StorageConstraints_[key] = newStorageConstraint(value)
		}
	}
	return app
}

// Tag implements Application.
func (s *application) Tag() names.ApplicationTag {
	return names.NewApplicationTag(s.Name_)
}

// Name implements Application.
func (s *application) Name() string {
	return s.Name_
}

// Series implements Application.
func (s *application) Series() string {
	return s.Series_
}

// Subordinate implements Application.
func (s *application) Subordinate() bool {
	return s.Subordinate_
}

// CharmURL implements Application.
func (s *application) CharmURL() string {
	return s.CharmURL_
}

// Channel implements Application.
func (s *application) Channel() string {
	return s.Channel_
}

// CharmModifiedVersion implements Application.
func (s *application) CharmModifiedVersion() int {
	return s.CharmModifiedVersion_
}

// ForceCharm implements Application.
func (s *application) ForceCharm() bool {
	return s.ForceCharm_
}

// Exposed implements Application.
func (s *application) Exposed() bool {
	return s.Exposed_
}

// MinUnits implements Application.
func (s *application) MinUnits() int {
	return s.MinUnits_
}

// Settings implements Application.
func (s *application) Settings() map[string]interface{} {
	return s.Settings_
}

// Leader implements Application.
func (s *application) Leader() string {
	return s.Leader_
}

// LeadershipSettings implements Application.
func (s *application) LeadershipSettings() map[string]interface{} {
	return s.LeadershipSettings_
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
func (s *application) MetricsCredentials() []byte {
	// Here we are explicitly throwing away any decode error. We check that
	// the creds can be decoded when we parse the incoming data, or we encode
	// an incoming byte array, so in both cases, we know that the stored creds
	// can be decoded.
	creds, _ := base64.StdEncoding.DecodeString(s.MetricsCredentials_)
	return creds
}

// Status implements Application.
func (s *application) Status() Status {
	// To avoid typed nils check nil here.
	if s.Status_ == nil {
		return nil
	}
	return s.Status_
}

// SetStatus implements Application.
func (s *application) SetStatus(args StatusArgs) {
	s.Status_ = newStatus(args)
}

// Units implements Application.
func (s *application) Units() []Unit {
	result := make([]Unit, len(s.Units_.Units_))
	for i, u := range s.Units_.Units_ {
		result[i] = u
	}
	return result
}

func (s *application) unitNames() set.Strings {
	result := set.NewStrings()
	for _, u := range s.Units_.Units_ {
		result.Add(u.Name())
	}
	return result
}

// AddUnit implements Application.
func (s *application) AddUnit(args UnitArgs) Unit {
	u := newUnit(args)
	s.Units_.Units_ = append(s.Units_.Units_, u)
	return u
}

func (s *application) setUnits(unitList []*unit) {
	s.Units_ = units{
		Version: 1,
		Units_:  unitList,
	}
}

// Constraints implements HasConstraints.
func (s *application) Constraints() Constraints {
	if s.Constraints_ == nil {
		return nil
	}
	return s.Constraints_
}

// SetConstraints implements HasConstraints.
func (s *application) SetConstraints(args ConstraintsArgs) {
	s.Constraints_ = newConstraints(args)
}

// Validate implements Application.
func (s *application) Validate() error {
	if s.Name_ == "" {
		return errors.NotValidf("application missing name")
	}
	if s.Status_ == nil {
		return errors.NotValidf("application %q missing status", s.Name_)
	}
	// If leader is set, it must match one of the units.
	var leaderFound bool
	// All of the applications units should also be valid.
	for _, u := range s.Units() {
		if err := u.Validate(); err != nil {
			return errors.Trace(err)
		}
		// We know that the unit has a name, because it validated correctly.
		if u.Name() == s.Leader_ {
			leaderFound = true
		}
	}
	if s.Leader_ != "" && !leaderFound {
		return errors.NotValidf("missing unit for leader %q", s.Leader_)
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
}

func importApplicationV1(source map[string]interface{}) (*application, error) {
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
		"settings":            schema.StringMap(schema.Any()),
		"leader":              schema.String(),
		"leadership-settings": schema.StringMap(schema.Any()),
		"storage-constraints": schema.StringMap(schema.StringMap(schema.Any())),
		"metrics-creds":       schema.String(),
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
	}
	addAnnotationSchema(fields, defaults)
	addConstraintsSchema(fields, defaults)
	addStatusHistorySchema(fields)
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "application v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &application{
		Name_:                 valid["name"].(string),
		Series_:               valid["series"].(string),
		Subordinate_:          valid["subordinate"].(bool),
		CharmURL_:             valid["charm-url"].(string),
		Channel_:              valid["cs-channel"].(string),
		CharmModifiedVersion_: int(valid["charm-mod-version"].(int64)),
		ForceCharm_:           valid["force-charm"].(bool),
		Exposed_:              valid["exposed"].(bool),
		MinUnits_:             int(valid["min-units"].(int64)),
		Settings_:             valid["settings"].(map[string]interface{}),
		Leader_:               valid["leader"].(string),
		LeadershipSettings_:   valid["leadership-settings"].(map[string]interface{}),
		StatusHistory_:        newStatusHistory(),
	}
	result.importAnnotations(valid)
	if err := result.importStatusHistory(valid); err != nil {
		return nil, errors.Trace(err)
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

	units, err := importUnits(valid["units"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.setUnits(units)

	return result, nil
}
