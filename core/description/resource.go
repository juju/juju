// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

// Resource represents an application resource.
type Resource interface {
	// Name returns the name of the resource.
	Name() string

	// SetApplicationRevision sets the application revision of the
	// resource.
	SetApplicationRevision(ResourceRevisionArgs) ResourceRevision

	// ApplicationRevision returns the revision of the resource as set
	// on the application. May return nil if SetApplicationRevision
	// hasn't been called yet.
	ApplicationRevision() ResourceRevision

	// SetCharmStoreRevision sets the application revision of the
	// resource.
	SetCharmStoreRevision(ResourceRevisionArgs) ResourceRevision

	// CharmStoreRevision returns the revision the charmstore has, as
	// seen at the last poll. May return nil if SetCharmStoreRevision
	// hasn't been called yet.
	CharmStoreRevision() ResourceRevision

	// Validate checks the consistency of the resource and its
	// revisions.
	Validate() error
}

// ResourceRevision represents a revision of an application resource.
type ResourceRevision interface {
	Revision() int
	Type() string
	Path() string
	Description() string
	Origin() string
	FingerprintHex() string
	Size() int64
	Timestamp() time.Time
	Username() string
}

// ResourceArgs is an argument struct used to create a new internal
// resource type that supports the Resource interface.
type ResourceArgs struct {
	Name string
}

// newResource returns a new *resource (which implements the Resource
// interface).
func newResource(args ResourceArgs) *resource {
	return &resource{
		Name_: args.Name,
	}
}

type resources struct {
	Version    int         `yaml:"version"`
	Resources_ []*resource `yaml:"resources"`
}

type resource struct {
	Name_                string            `yaml:"name"`
	ApplicationRevision_ *resourceRevision `yaml:"application-revision"`
	CharmStoreRevision_  *resourceRevision `yaml:"charmstore-revision,omitempty"`
}

// ResourceRevisionArgs is an argument struct used to add a new
// internal resource revision to a Resource.
type ResourceRevisionArgs struct {
	Revision       int
	Type           string
	Path           string
	Description    string
	Origin         string
	FingerprintHex string
	Size           int64
	Timestamp      time.Time
	Username       string
}

// Name implements Resource.
func (r *resource) Name() string {
	return r.Name_
}

// SetApplicationRevision implements Resource.
func (r *resource) SetApplicationRevision(args ResourceRevisionArgs) ResourceRevision {
	r.ApplicationRevision_ = newResourceRevision(args)
	return r.ApplicationRevision_
}

// ApplicationRevision implements Resource.
func (r *resource) ApplicationRevision() ResourceRevision {
	if r.ApplicationRevision_ == nil {
		return nil // Return untyped nil when not set
	}
	return r.ApplicationRevision_
}

// SetCharmStoreRevision implements Resource.
func (r *resource) SetCharmStoreRevision(args ResourceRevisionArgs) ResourceRevision {
	r.CharmStoreRevision_ = newResourceRevision(args)
	return r.CharmStoreRevision_
}

// CharmStoreRevision implements Resource.
func (r *resource) CharmStoreRevision() ResourceRevision {
	if r.CharmStoreRevision_ == nil {
		return nil // Return untyped nil when not set
	}
	return r.CharmStoreRevision_
}

// Validate implements Resource.
func (r *resource) Validate() error {
	if r.ApplicationRevision_ == nil {
		return errors.New("no application revision set")
	}
	return nil
}

func newResourceRevision(args ResourceRevisionArgs) *resourceRevision {
	return &resourceRevision{
		Revision_:       args.Revision,
		Type_:           args.Type,
		Path_:           args.Path,
		Description_:    args.Description,
		Origin_:         args.Origin,
		FingerprintHex_: args.FingerprintHex,
		Size_:           args.Size,
		Timestamp_:      timePtr(args.Timestamp),
		Username_:       args.Username,
	}
}

type resourceRevision struct {
	Revision_       int        `yaml:"revision"`
	Type_           string     `yaml:"type"`
	Path_           string     `yaml:"path"`
	Description_    string     `yaml:"description"`
	Origin_         string     `yaml:"origin"`
	FingerprintHex_ string     `yaml:"fingerprint"`
	Size_           int64      `yaml:"size"`
	Timestamp_      *time.Time `yaml:"timestamp,omitempty"`
	Username_       string     `yaml:"username,omitempty"`
}

// Revision implements ResourceRevision.
func (r *resourceRevision) Revision() int {
	return r.Revision_
}

// Type implements ResourceRevision.
func (r *resourceRevision) Type() string {
	return r.Type_
}

// Path implements ResourceRevision.
func (r *resourceRevision) Path() string {
	return r.Path_
}

// Description implements ResourceRevision.
func (r *resourceRevision) Description() string {
	return r.Description_
}

// Origin implements ResourceRevision.
func (r *resourceRevision) Origin() string {
	return r.Origin_
}

// FingerprintHex implements ResourceRevision.
func (r *resourceRevision) FingerprintHex() string {
	return r.FingerprintHex_
}

// Size implements ResourceRevision.
func (r *resourceRevision) Size() int64 {
	return r.Size_
}

// Timestamp implements ResourceRevision.
func (r *resourceRevision) Timestamp() time.Time {
	if r.Timestamp_ == nil {
		return time.Time{}
	}
	return *r.Timestamp_
}

// Username implements ResourceRevision.
func (r *resourceRevision) Username() string {
	return r.Username_
}

func importResources(source map[string]interface{}) ([]*resource, error) {
	checker := versionedChecker("resources")
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotate(err, "resources version schema check failed")
	}
	valid := coerced.(map[string]interface{})

	version := int(valid["version"].(int64))
	importFunc, ok := resourceDeserializationFuncs[version]
	if !ok {
		return nil, errors.NotValidf("version %d", version)
	}
	sourceList := valid["resources"].([]interface{})
	return importResourceList(sourceList, importFunc)
}

func importResourceList(sourceList []interface{}, importFunc resourceDeserializationFunc) ([]*resource, error) {
	result := make([]*resource, 0, len(sourceList))
	for i, value := range sourceList {
		source, ok := value.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("unexpected value for resource %d, %T", i, value)
		}
		resource, err := importFunc(source)
		if err != nil {
			return nil, errors.Annotatef(err, "resource %d", i)
		}
		result = append(result, resource)
	}
	return result, nil
}

type resourceDeserializationFunc func(map[string]interface{}) (*resource, error)

var resourceDeserializationFuncs = map[int]resourceDeserializationFunc{
	1: importResourceV1,
}

func importResourceV1(source map[string]interface{}) (*resource, error) {
	fields := schema.Fields{
		"name":                 schema.String(),
		"application-revision": schema.StringMap(schema.Any()),
		"charmstore-revision":  schema.StringMap(schema.Any()),
	}
	defaults := schema.Defaults{
		"charmstore-revision": schema.Omit,
	}
	checker := schema.FieldMap(fields, defaults)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "resource v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})

	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	r := newResource(ResourceArgs{
		Name: valid["name"].(string),
	})
	appRev, err := importResourceRevisionV1(valid["application-revision"])
	if err != nil {
		return nil, errors.Annotatef(err, "resource %s: application revision", r.Name_)
	}
	r.ApplicationRevision_ = appRev
	if source, exists := valid["charmstore-revision"]; exists {
		csRev, err := importResourceRevisionV1(source)
		if err != nil {
			return nil, errors.Annotatef(err, "resource %s: charmstore revision", r.Name_)
		}
		r.CharmStoreRevision_ = csRev
	}
	return r, nil
}

func importResourceRevisionV1(source interface{}) (*resourceRevision, error) {
	fields := schema.Fields{
		"revision":    schema.Int(),
		"type":        schema.String(),
		"path":        schema.String(),
		"description": schema.String(),
		"origin":      schema.String(),
		"fingerprint": schema.String(),
		"size":        schema.Int(),
		"timestamp":   schema.Time(),
		"username":    schema.String(),
	}
	defaults := schema.Defaults{
		"timestamp": schema.Omit,
		"username":  "",
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "resource revision schema check failed")
	}
	valid := coerced.(map[string]interface{})

	rev := &resourceRevision{
		Revision_:       int(valid["revision"].(int64)),
		Type_:           valid["type"].(string),
		Path_:           valid["path"].(string),
		Description_:    valid["description"].(string),
		Origin_:         valid["origin"].(string),
		FingerprintHex_: valid["fingerprint"].(string),
		Size_:           valid["size"].(int64),
		Timestamp_:      fieldToTimePtr(valid, "timestamp"),
		Username_:       valid["username"].(string),
	}
	return rev, nil
}
