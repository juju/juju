// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/set"
)

type relations struct {
	Version    int         `yaml:"version"`
	Relations_ []*relation `yaml:"relations"`
}

type relation struct {
	Id_        int        `yaml:"id"`
	Key_       string     `yaml:"key"`
	EndPoints_ *endpoints `yaml:"endpoints"`
}

// RelationArgs is an argument struct used to specify a relation.
type RelationArgs struct {
	Id  int
	Key string
}

func newRelation(args RelationArgs) *relation {
	relation := &relation{
		Id_:  args.Id,
		Key_: args.Key,
	}
	relation.setEndPoints(nil)
	return relation
}

// Id implements Relation.
func (r *relation) Id() int {
	return r.Id_
}

// Key implements Relation.
func (r *relation) Key() string {
	return r.Key_
}

// EndPoints implements Relation.
func (r *relation) EndPoints() []EndPoint {
	result := make([]EndPoint, len(r.EndPoints_.EndPoints_))
	for i, ep := range r.EndPoints_.EndPoints_ {
		result[i] = ep
	}
	return result
}

// AddEndpoint implements Relation.
func (r *relation) AddEndpoint(args EndPointArgs) EndPoint {
	ep := newEndPoint(args)
	r.EndPoints_.EndPoints_ = append(r.EndPoints_.EndPoints_, ep)
	return ep
}

func (r *relation) setEndPoints(endpointList []*endpoint) {
	r.EndPoints_ = &endpoints{
		Version:    1,
		EndPoints_: endpointList,
	}
}

func importRelations(source map[string]interface{}) ([]*relation, error) {
	checker := versionedChecker("relations")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "relations version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := relationDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	relationList := valid["relations"].([]interface{})
	return importRelationList(relationList, importFunc)
}

func importRelationList(sourceList []interface{}, importFunc relationDeserializationFunc) ([]*relation, error) {
	result := make([]*relation, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for relation %d, %T", i, value)
		}
		relation, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "relation %d", i)
		}
		result = append(result, relation)
	}
	return result, nil
}

type relationDeserializationFunc func(map[string]interface{}) (*relation, error)

var relationDeserializationFuncs = map[int]relationDeserializationFunc{
	1: importRelationV1,
}

func importRelationV1(source map[string]interface{}) (*relation, error) {
	fields := schema.Fields{
		"id":        schema.Int(),
		"key":       schema.String(),
		"endpoints": schema.StringMap(schema.Any()),
	}

	checker := schema.FieldMap(fields, nil) // no defaults

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "relation v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &relation{
		Id_:  int(valid["id"].(int64)),
		Key_: valid["key"].(string),
	}

	endpoints, err := importEndPoints(valid["endpoints"].(map[string]interface{}))
	if err != nil {
		return nil, errors.Trace(err)
	}
	result.setEndPoints(endpoints)

	return result, nil
}

type endpoints struct {
	Version    int         `yaml:"version"`
	EndPoints_ []*endpoint `yaml:"endpoints"`
}

type endpoint struct {
	ServiceName_ string `yaml:"service-name"`
	Name_        string `yaml:"name"`
	Role_        string `yaml:"role"`
	Interface_   string `yaml:"interface"`
	Optional_    bool   `yaml:"optional"`
	Limit_       int    `yaml:"limit"`
	Scope_       string `yaml:"scope"`

	UnitSettings_ map[string]map[string]interface{} `yaml:"unit-settings"`
}

// EndPointArgs is an argument struct used to specify a relation.
type EndPointArgs struct {
	ServiceName string
	Name        string
	Role        string
	Interface   string
	Optional    bool
	Limit       int
	Scope       string
}

func newEndPoint(args EndPointArgs) *endpoint {
	return &endpoint{
		ServiceName_:  args.ServiceName,
		Name_:         args.Name,
		Role_:         args.Role,
		Interface_:    args.Interface,
		Optional_:     args.Optional,
		Limit_:        args.Limit,
		Scope_:        args.Scope,
		UnitSettings_: make(map[string]map[string]interface{}),
	}
}

func (e *endpoint) unitNames() set.Strings {
	result := set.NewStrings()
	for key := range e.UnitSettings_ {
		result.Add(key)
	}
	return result
}

// ServiceName implements EndPoint.
func (e *endpoint) ServiceName() string {
	return e.ServiceName_
}

// Name implements EndPoint.
func (e *endpoint) Name() string {
	return e.Name_
}

// Role implements EndPoint.
func (e *endpoint) Role() string {
	return e.Role_
}

// Interface implements EndPoint.
func (e *endpoint) Interface() string {
	return e.Interface_
}

// Optional implements EndPoint.
func (e *endpoint) Optional() bool {
	return e.Optional_
}

// Limit implements EndPoint.
func (e *endpoint) Limit() int {
	return e.Limit_
}

// Scope implements EndPoint.
func (e *endpoint) Scope() string {
	return e.Scope_
}

// Settings implements EndPoint.
func (e *endpoint) Settings(unitName string) map[string]interface{} {
	return e.UnitSettings_[unitName]
}

// SetUnitSettings implements EndPoint.
func (e *endpoint) SetUnitSettings(unitName string, settings map[string]interface{}) {
	e.UnitSettings_[unitName] = settings
}

func importEndPoints(source map[string]interface{}) ([]*endpoint, error) {
	checker := versionedChecker("endpoints")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "endpoints version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := endpointDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	endpointList := valid["endpoints"].([]interface{})
	return importEndPointList(endpointList, importFunc)
}

func importEndPointList(sourceList []interface{}, importFunc endpointDeserializationFunc) ([]*endpoint, error) {
	result := make([]*endpoint, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for endpoint %d, %T", i, value)
		}
		service, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "endpoint %d", i)
		}
		result = append(result, service)
	}
	return result, nil
}

type endpointDeserializationFunc func(map[string]interface{}) (*endpoint, error)

var endpointDeserializationFuncs = map[int]endpointDeserializationFunc{
	1: importEndPointV1,
}

func importEndPointV1(source map[string]interface{}) (*endpoint, error) {
	fields := schema.Fields{
		"service-name":  schema.String(),
		"name":          schema.String(),
		"role":          schema.String(),
		"interface":     schema.String(),
		"optional":      schema.Bool(),
		"limit":         schema.Int(),
		"scope":         schema.String(),
		"unit-settings": schema.StringMap(schema.StringMap(schema.Any())),
	}

	checker := schema.FieldMap(fields, nil) // No defaults.

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "endpoint v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.

	result := &endpoint{
		ServiceName_:  valid["service-name"].(string),
		Name_:         valid["name"].(string),
		Role_:         valid["role"].(string),
		Interface_:    valid["interface"].(string),
		Optional_:     valid["optional"].(bool),
		Limit_:        int(valid["limit"].(int64)),
		Scope_:        valid["scope"].(string),
		UnitSettings_: make(map[string]map[string]interface{}),
	}

	for unitname, settings := range valid["unit-settings"].(map[string]interface{}) {
		result.UnitSettings_[unitname] = settings.(map[string]interface{})
	}

	return result, nil
}
