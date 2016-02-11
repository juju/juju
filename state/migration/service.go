// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/schema"
)

type services struct {
	Version   int        `yaml:"version"`
	Services_ []*service `yaml:"services"`
}

type service struct {
	Name_        string `yaml:"name"`
	Series_      string `yaml:"series"`
	Subordinate_ bool   `yaml:"subordinate,omitempty"`
	CharmURL_    string `yaml:"charm-url"`
	// ForceCharm is true if an upgrade charm is forced.
	// It means upgrade even if the charm is in an error state.
	ForceCharm_ bool    `yaml:"force-charm,omitempty"`
	Exposed_    bool    `yaml:"exposed,omitempty"`
	MinUnits_   int     `yaml:"min-units,omitempty"`
	Status_     *status `yaml:"status"`

	Settings_           map[string]interface{} `yaml:"settings"`
	SettingsRefCount_   int                    `yaml:"settings-refcount"`
	LeadershipSettings_ map[string]interface{} `yaml:"leadership-settings"`

	// unit count will be assumed by the number of units associated.
	Units_ units `yaml:"units"`

	// relation count also assumed by the relation sequence

	// TODO: confirm with casey if these are needed
	//	MetricsCredentials []byte `yaml:"metrics-creds"`

	// Requested Networks
	// Constraints
	// Storage Constraints
	// Annotations
	// Status history
}

// ServiceArgs is an argument struct used to add a service to the Model.
type ServiceArgs struct {
	Tag                names.ServiceTag
	Series             string
	Subordinate        bool
	CharmURL           string
	ForceCharm         bool
	Exposed            bool
	MinUnits           int
	Settings           map[string]interface{}
	SettingsRefCount   int
	LeadershipSettings map[string]interface{}
}

func newService(args ServiceArgs) *service {
	svc := &service{
		Name_:               args.Tag.Id(),
		Series_:             args.Series,
		Subordinate_:        args.Subordinate,
		CharmURL_:           args.CharmURL,
		ForceCharm_:         args.ForceCharm,
		Exposed_:            args.Exposed,
		MinUnits_:           args.MinUnits,
		Settings_:           args.Settings,
		SettingsRefCount_:   args.SettingsRefCount,
		LeadershipSettings_: args.LeadershipSettings,
	}
	svc.setUnits(nil)
	return svc
}

// Tag impelements Service.
func (s *service) Tag() names.ServiceTag {
	return names.NewServiceTag(s.Name_)
}

// Name impelements Service.
func (s *service) Name() string {
	return s.Name_
}

// Series impelements Service.
func (s *service) Series() string {
	return s.Series_
}

// Subordinate impelements Service.
func (s *service) Subordinate() bool {
	return s.Subordinate_
}

// CharmURL impelements Service.
func (s *service) CharmURL() string {
	return s.CharmURL_
}

// ForceCharm impelements Service.
func (s *service) ForceCharm() bool {
	return s.ForceCharm_
}

// Exposed impelements Service.
func (s *service) Exposed() bool {
	return s.Exposed_
}

// MinUnits impelements Service.
func (s *service) MinUnits() int {
	return s.MinUnits_
}

// Settings impelements Service.
func (s *service) Settings() map[string]interface{} {
	return s.Settings_
}

// SettingsRefCount impelements Service.
func (s *service) SettingsRefCount() int {
	return s.SettingsRefCount_
}

// LeadershipSettings impelements Service.
func (s *service) LeadershipSettings() map[string]interface{} {
	return s.LeadershipSettings_
}

// Status impelements Service.
func (s *service) Status() Status {
	// To avoid typed nils check nil here.
	if s.Status_ == nil {
		return nil
	}
	return s.Status_
}

// SetStatus impelements Service.
func (s *service) SetStatus(args StatusArgs) {
	s.Status_ = newStatus(args)
}

// Units impelements Service.
func (s *service) Units() []Unit {
	result := make([]Unit, len(s.Units_.Units_))
	for i, u := range s.Units_.Units_ {
		result[i] = u
	}
	return result
}

// AddUnit impelements Service.
func (s *service) AddUnit(args UnitArgs) Unit {
	u := newUnit(args)
	s.Units_.Units_ = append(s.Units_.Units_, u)
	return u
}

func (s *service) setUnits(unitList []*unit) {
	s.Units_ = units{
		Version: 1,
		Units_:  unitList,
	}
}

// Validate impelements Service.
func (s *service) Validate() error {
	if s.Name_ == "" {
		return errors.NotValidf("missing name")
	}
	if s.Status_ == nil {
		return errors.NotValidf("service %q missing status", s.Name_)
	}
	// All of the services units should also be valid.
	for _, u := range s.Units() {
		if err := u.Validate(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func importServices(source map[string]interface{}) ([]*service, error) {
	checker := versionedChecker("services")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "services version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := serviceDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["services"].([]interface{})
	return importServiceList(sourceList, importFunc)
}

func importServiceList(sourceList []interface{}, importFunc serviceDeserializationFunc) ([]*service, error) {
	result := make([]*service, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for service %d, %T", i, value)
		}
		service, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "service %d", i)
		}
		result = append(result, service)
	}
	return result, nil
}

type serviceDeserializationFunc func(map[string]interface{}) (*service, error)

var serviceDeserializationFuncs = map[int]serviceDeserializationFunc{
	1: importServiceV1,
}

func importServiceV1(source map[string]interface{}) (*service, error) {
	result := &service{}

	fields := schema.Fields{
		"name":                schema.String(),
		"series":              schema.String(),
		"subordinate":         schema.Bool(),
		"charm-url":           schema.String(),
		"force-charm":         schema.Bool(),
		"exposed":             schema.Bool(),
		"min-units":           schema.Int(),
		"status":              schema.StringMap(schema.Any()),
		"settings":            schema.StringMap(schema.Any()),
		"settings-refcount":   schema.Int(),
		"leadership-settings": schema.StringMap(schema.Any()),
		"units":               schema.StringMap(schema.Any()),
	}

	defaults := schema.Defaults{
		"subordinate": false,
		"force-charm": false,
		"exposed":     false,
		"min-units":   int64(0),
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "service v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result.Name_ = valid["name"].(string)
	result.Series_ = valid["series"].(string)
	result.Subordinate_ = valid["subordinate"].(bool)
	result.CharmURL_ = valid["charm-url"].(string)
	result.ForceCharm_ = valid["force-charm"].(bool)
	result.Exposed_ = valid["exposed"].(bool)
	result.MinUnits_ = int(valid["min-units"].(int64))

	result.Settings_ = valid["settings"].(map[string]interface{})
	result.SettingsRefCount_ = int(valid["settings-refcount"].(int64))
	result.LeadershipSettings_ = valid["leadership-settings"].(map[string]interface{})

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
