// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
)

// Resource represents an application resource.
type Resource interface {
	Name() string
	Revision() int
	CharmStoreRevision() int
	AddRevision(ResourceRevisionArgs)
	Revisions() map[int]ResourceRevision
	Validate() error
}

// ResourceRevision represents a revision of an application resource.
type ResourceRevision interface {
	Revision() int
	Type() string
	Path() string
	Description() string
	Origin() string
	Fingerprint() string
	Size() int64
	AddTimestamp() time.Time
	Username() string
}

// ResourceArgs is an argument struct used to create a new internal
// resource type that supports the Resource interface.
type ResourceArgs struct {
	Name               string
	Revision           int
	CharmStoreRevision int
}

func newResource(args ResourceArgs) *resource {
	return &resource{
		Name_:               args.Name,
		Revision_:           args.Revision,
		CharmStoreRevision_: args.CharmStoreRevision,
		Revisions_:          make(map[string]ResourceRevision),
	}
}

type resources struct {
	Version    int         `yaml:"version"`
	Resources_ []*resource `yaml:"resources"`
}

type resource struct {
	Name_               string `yaml:"name"`
	Revision_           int    `yaml:"application-revision"`
	CharmStoreRevision_ int    `yaml:"charmstore-revision"`
	// Revisions_ uses string keys to avoid serialisation problems.
	// XXX make this a type with it's own serialisation?
	Revisions_ map[string]ResourceRevision `yaml:"revisions"`
}

/*
type ResourceRevisionMap map[int]ResourceRevision

// MarshalYAML implements yaml.v2.Marshaller interface.
func (m ResourceRevisionMap) MarshalYAML() (interface{}, error) {
	return b.String(), nil
}
*/

// Name implements Resource.
func (r *resource) Name() string {
	return r.Name_
}

// Revision implements Resource.
func (r *resource) Revision() int {
	return r.Revision_
}

// CharmStoreRevision implements Resource.
func (r *resource) CharmStoreRevision() int {
	return r.CharmStoreRevision_
}

// ResourceArgs is an argument struct used to add a new internal
// resource revision to a Resource.
type ResourceRevisionArgs struct {
	Revision     int
	Type         string
	Path         string
	Description  string
	Origin       string
	Fingerprint  string
	Size         int64
	AddTimestamp time.Time
	Username     string
}

// AddRevision implements Resource.
func (r *resource) AddRevision(args ResourceRevisionArgs) {
	var addTs *time.Time
	if !args.AddTimestamp.IsZero() {
		t := args.AddTimestamp.UTC()
		addTs = &t
	}
	rev := &resourceRevision{
		Revision_:     args.Revision,
		Type_:         args.Type,
		Path_:         args.Path,
		Description_:  args.Description,
		Origin_:       args.Origin,
		Fingerprint_:  args.Fingerprint,
		Size_:         args.Size,
		AddTimestamp_: addTs,
		Username_:     args.Username,
	}
	r.Revisions_[strconv.Itoa(args.Revision)] = rev
}

// Revisions implements Resource.
func (r *resource) Revisions() map[int]ResourceRevision {
	out := make(map[int]ResourceRevision)
	for k, v := range r.Revisions_ {
		revId, err := strconv.Atoi(k)
		if err != nil {
			// XXX
			panic(err) // This really is very unlikely.
		}
		out[revId] = v
	}
	return out
}

// Validate implements Resource.
func (r *resource) Validate() error {
	if _, ok := r.Revisions_[strconv.Itoa(r.Revision_)]; !ok {
		return errors.Errorf("missing application revision (%d)", r.Revision_)
	}
	if _, ok := r.Revisions_[strconv.Itoa(r.CharmStoreRevision_)]; !ok {
		return errors.Errorf("missing charmstore revision (%d)", r.CharmStoreRevision_)
	}
	return nil
}

type resourceRevision struct {
	Revision_     int        `yaml:"revision"`
	Type_         string     `yaml:"type"`
	Path_         string     `yaml:"path"`
	Description_  string     `yaml:"description"`
	Origin_       string     `yaml:"origin"`
	Fingerprint_  string     `yaml:"fingerprint"` // XXX include Hex in the name?
	Size_         int64      `yaml:"size"`
	AddTimestamp_ *time.Time `yaml:"add-timestamp,omitempty"`
	Username_     string     `yaml:"username,omitempty"`
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

// Fingerprint implements ResourceRevision.
func (r *resourceRevision) Fingerprint() string {
	return r.Fingerprint_
}

// Size implements ResourceRevision.
func (r *resourceRevision) Size() int64 {
	return r.Size_
}

// AddTimestamp implements ResourceRevision.
func (r *resourceRevision) AddTimestamp() time.Time {
	if r.AddTimestamp_ == nil {
		return time.Time{}
	}
	return *r.AddTimestamp_
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
		"application-revision": schema.Int(),
		"charmstore-revision":  schema.Int(),
		"revisions":            schema.StringMap(schema.Any()),
	}
	checker := schema.FieldMap(fields, nil)

	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "resource v1 schema check failed")
	}
	valid := coerced.(map[string]interface{})

	// From here we know that the map returned from the schema coercion
	// contains fields of the right type.
	name := valid["name"].(string)
	r := &resource{
		Name_:               name,
		Revision_:           int(valid["application-revision"].(int64)),
		CharmStoreRevision_: int(valid["charmstore-revision"].(int64)),
		Revisions_:          make(map[string]ResourceRevision),
	}

	// Now read in the resource revisions...
	revsMap := valid["revisions"].(map[string]interface{})
	for revNum, revSource := range revsMap {
		revision, err := importResourceRevisionV1(revSource)
		if err != nil {
			return nil, errors.Annotatef(err, "resource %s, revision %s", name, revNum)
		}
		r.Revisions_[revNum] = revision
	}

	return r, nil
}

func importResourceRevisionV1(source interface{}) (*resourceRevision, error) {
	fields := schema.Fields{
		"revision":      schema.Int(),
		"type":          schema.String(),
		"path":          schema.String(),
		"description":   schema.String(),
		"origin":        schema.String(),
		"fingerprint":   schema.String(),
		"size":          schema.Int(),
		"add-timestamp": schema.Time(),
		"username":      schema.String(),
	}
	defaults := schema.Defaults{
		"add-timestamp": time.Time{},
		"username":      "",
	}
	checker := schema.FieldMap(fields, defaults)
	coerced, err := checker.Coerce(source, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "resource revision schema check failed")
	}
	valid := coerced.(map[string]interface{})

	rev := &resourceRevision{
		Revision_:    int(valid["revision"].(int64)),
		Type_:        valid["type"].(string),
		Path_:        valid["path"].(string),
		Description_: valid["description"].(string),
		Origin_:      valid["origin"].(string),
		Fingerprint_: valid["fingerprint"].(string),
		Size_:        valid["size"].(int64),
		Username_:    valid["username"].(string),
	}
	addTs := valid["add-timestamp"].(time.Time)
	if !addTs.IsZero() {
		rev.AddTimestamp_ = &addTs
	}
	return rev, nil
}
