// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package description

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/names.v2"
)

// RemoteApplication represents an application in another model that
// can participate in a relation in this model.
type RemoteApplication interface {
	HasStatus

	Tag() names.ApplicationTag
	Name() string
	OfferUUID() string
	URL() string
	SourceModelTag() names.ModelTag
	IsConsumerProxy() bool

	Endpoints() []RemoteEndpoint
	AddEndpoint(RemoteEndpointArgs) RemoteEndpoint

	Spaces() []RemoteSpace
	AddSpace(RemoteSpaceArgs) RemoteSpace

	Bindings() map[string]string
}

type remoteApplications struct {
	Version            int                  `yaml:"version"`
	RemoteApplications []*remoteApplication `yaml:"remote-applications"`
}

type remoteApplication struct {
	Name_            string            `yaml:"name"`
	OfferUUID_       string            `yaml:"offer-uuid"`
	URL_             string            `yaml:"url"`
	SourceModelUUID_ string            `yaml:"source-model-uuid"`
	Endpoints_       remoteEndpoints   `yaml:"endpoints,omitempty"`
	IsConsumerProxy_ bool              `yaml:"is-consumer-proxy,omitempty"`
	Spaces_          remoteSpaces      `yaml:"spaces,omitempty"`
	Bindings_        map[string]string `yaml:"bindings,omitempty"`
	Status_          *status           `yaml:"status"`
}

// RemoteApplicationArgs is an argument struct used to add a remote
// application to the Model.
type RemoteApplicationArgs struct {
	Tag             names.ApplicationTag
	OfferUUID       string
	URL             string
	SourceModel     names.ModelTag
	IsConsumerProxy bool
	Bindings        map[string]string
}

func newRemoteApplication(args RemoteApplicationArgs) *remoteApplication {
	a := &remoteApplication{
		Name_:            args.Tag.Id(),
		OfferUUID_:       args.OfferUUID,
		URL_:             args.URL,
		SourceModelUUID_: args.SourceModel.Id(),
		IsConsumerProxy_: args.IsConsumerProxy,
		Bindings_:        args.Bindings,
	}
	a.setEndpoints(nil)
	a.setSpaces(nil)
	return a
}

// Tag implements RemoteApplication.
func (a *remoteApplication) Tag() names.ApplicationTag {
	return names.NewApplicationTag(a.Name_)
}

// Name implements RemoteApplication.
func (a *remoteApplication) Name() string {
	return a.Name_
}

// OfferUUID implements RemoteApplication.
func (a *remoteApplication) OfferUUID() string {
	return a.OfferUUID_
}

// URL implements RemoteApplication.
func (a *remoteApplication) URL() string {
	return a.URL_
}

// SourceModelTag implements RemoteApplication.
func (a *remoteApplication) SourceModelTag() names.ModelTag {
	return names.NewModelTag(a.SourceModelUUID_)
}

// IsConsumerProxy implements RemoteApplication.
func (a *remoteApplication) IsConsumerProxy() bool {
	return a.IsConsumerProxy_
}

// Bindings implements RemoteApplication.
func (a *remoteApplication) Bindings() map[string]string {
	return a.Bindings_
}

// Status implements RemoteApplication.
func (a *remoteApplication) Status() Status {
	// Avoid typed nils.
	if a.Status_ == nil {
		return nil
	}
	return a.Status_
}

// SetStatus implements RemoteApplication.
func (a *remoteApplication) SetStatus(args StatusArgs) {
	a.Status_ = newStatus(args)
}

// Endpoints implements RemoteApplication.
func (a *remoteApplication) Endpoints() []RemoteEndpoint {
	result := make([]RemoteEndpoint, len(a.Endpoints_.Endpoints))
	for i, endpoint := range a.Endpoints_.Endpoints {
		result[i] = endpoint
	}
	return result
}

// AddEndpoint implements RemoteApplication.
func (a *remoteApplication) AddEndpoint(args RemoteEndpointArgs) RemoteEndpoint {
	ep := newRemoteEndpoint(args)
	a.Endpoints_.Endpoints = append(a.Endpoints_.Endpoints, ep)
	return ep
}

func (a *remoteApplication) setEndpoints(endpointList []*remoteEndpoint) {
	a.Endpoints_ = remoteEndpoints{
		Version:   1,
		Endpoints: endpointList,
	}
}

// Spaces implements RemoteApplication.
func (a *remoteApplication) Spaces() []RemoteSpace {
	result := make([]RemoteSpace, len(a.Spaces_.Spaces))
	for i, space := range a.Spaces_.Spaces {
		result[i] = space
	}
	return result
}

// AddSpace implements RemoteApplication.
func (a *remoteApplication) AddSpace(args RemoteSpaceArgs) RemoteSpace {
	ep := newRemoteSpace(args)
	a.Spaces_.Spaces = append(a.Spaces_.Spaces, ep)
	return ep
}

func (a *remoteApplication) setSpaces(spaceList []*remoteSpace) {
	a.Spaces_ = remoteSpaces{
		Version: 1,
		Spaces:  spaceList,
	}
}

func importRemoteApplications(source interface{}) ([]*remoteApplication, error) {
	checker := versionedChecker("remote-applications")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "remote applications version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	getFields, ok := remoteApplicationFieldsFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["remote-applications"].([]interface{})
	return importRemoteApplicationList(sourceList, schema.FieldMap(getFields()), version)
}

func importRemoteApplicationList(sourceList []interface{}, checker schema.Checker, version int) ([]*remoteApplication, error) {
	var result []*remoteApplication
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for remote application %d, %T", i, value)
		}
		coerced, err := checker.Coerce(source, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "remote application %d v%d schema check failed", i, version)
		}
		valid := coerced.(map[string]interface{})
		remoteApp, err := newRemoteApplicationFromValid(valid, version)
		if err != nil {
			return nil, errors.Annotatef(err, "remote application %d", i)
		}
		result = append(result, remoteApp)
	}
	return result, nil
}

var remoteApplicationFieldsFuncs = map[int]fieldsFunc{
	1: remoteApplicationV1Fields,
}

func newRemoteApplicationFromValid(valid map[string]interface{}, version int) (*remoteApplication, error) {
	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	result := &remoteApplication{
		Name_:            valid["name"].(string),
		OfferUUID_:       valid["offer-uuid"].(string),
		URL_:             valid["url"].(string),
		SourceModelUUID_: valid["source-model-uuid"].(string),
		IsConsumerProxy_: valid["is-consumer-proxy"].(bool),
	}

	if rawStatus := valid["status"]; rawStatus != nil {
		status, err := importStatus(rawStatus.(map[string]interface{}))
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.Status_ = status
	}

	if rawEndpoints, ok := valid["endpoints"]; ok {
		endpointsMap := rawEndpoints.(map[string]interface{})
		endpoints, err := importRemoteEndpoints(endpointsMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.setEndpoints(endpoints)
	}
	if rawSpaces, ok := valid["spaces"]; ok {
		spacesMap := rawSpaces.(map[string]interface{})
		spaces, err := importRemoteSpaces(spacesMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result.setSpaces(spaces)
	}
	if bindings, ok := valid["bindings"]; ok {
		result.Bindings_ = convertToStringMap(bindings)
	}
	return result, nil
}

func remoteApplicationV1Fields() (schema.Fields, schema.Defaults) {
	fields := schema.Fields{
		"name":              schema.String(),
		"offer-uuid":        schema.String(),
		"url":               schema.String(),
		"source-model-uuid": schema.String(),
		"status":            schema.StringMap(schema.Any()),
		"endpoints":         schema.StringMap(schema.Any()),
		"is-consumer-proxy": schema.Bool(),
		"spaces":            schema.StringMap(schema.Any()),
		"bindings":          schema.StringMap(schema.String()),
	}

	defaults := schema.Defaults{
		"endpoints":         schema.Omit,
		"spaces":            schema.Omit,
		"bindings":          schema.Omit,
		"is-consumer-proxy": false,
	}
	return fields, defaults
}
