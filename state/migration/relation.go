// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
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
