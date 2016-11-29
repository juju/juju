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

	// ApplicationRevision returns the revision of the resource as set
	// on the application.
	ApplicationRevision() int

	// CharmStoreRevision returns the revision the charmstore has, as
	// seen at the last poll.
	CharmStoreRevision() int

	// AddRevision defines a revision of the resource. If the revision
	// has already been added, the revision details will be merged
	// with the details added before.
	AddRevision(ResourceRevisionArgs) (ResourceRevision, error)

	// Revisions returns a map of known revisions for the resource,
	// indexed by the revision number.
	Revisions() map[int]ResourceRevision

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
	Name               string
	Revision           int
	CharmStoreRevision int
}

func newResource(args ResourceArgs) *resource {
	return &resource{
		Name_:                args.Name,
		ApplicationRevision_: args.Revision,
		CharmStoreRevision_:  args.CharmStoreRevision,
	}
}

type resources struct {
	Version    int         `yaml:"version"`
	Resources_ []*resource `yaml:"resources"`
}

type resource struct {
	Name_                string              `yaml:"name"`
	ApplicationRevision_ int                 `yaml:"application-revision"`
	CharmStoreRevision_  int                 `yaml:"charmstore-revision"`
	Revisions_           []*resourceRevision `yaml:"revisions"`
}

// Name implements Resource.
func (r *resource) Name() string {
	return r.Name_
}

// ApplicationRevision implements Resource.
func (r *resource) ApplicationRevision() int {
	return r.ApplicationRevision_
}

// CharmStoreRevision implements Resource.
func (r *resource) CharmStoreRevision() int {
	return r.CharmStoreRevision_
}

// ResourceArgs is an argument struct used to add a new internal
// resource revision to a Resource.
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

// AddRevision implements Resource.
func (r *resource) AddRevision(args ResourceRevisionArgs) (ResourceRevision, error) {
	for _, rev := range r.Revisions_ {
		if rev.Revision_ == args.Revision {
			return rev.merge(args)
		}
	}
	rev := &resourceRevision{
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
	r.Revisions_ = append(r.Revisions_, rev)
	return rev, nil
}

// Revisions implements Resource.
func (r *resource) Revisions() map[int]ResourceRevision {
	out := make(map[int]ResourceRevision)
	for _, rev := range r.Revisions_ {
		out[rev.Revision_] = rev
	}
	return out
}

// Validate implements Resource.
func (r *resource) Validate() error {
	revs := r.Revisions()
	if _, ok := revs[r.ApplicationRevision_]; !ok {
		return errors.Errorf("missing application revision (%d)", r.ApplicationRevision_)
	}
	if _, ok := revs[r.CharmStoreRevision_]; !ok {
		return errors.Errorf("missing charmstore revision (%d)", r.CharmStoreRevision_)
	}

	seenRevs := make(map[int]struct{})
	for _, rev := range r.Revisions_ {
		revNum := rev.Revision_
		if _, exists := seenRevs[revNum]; exists {
			return errors.Errorf("revision %d appears more than once", revNum)
		}
		seenRevs[revNum] = struct{}{}
	}

	return nil
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

func (r *resourceRevision) merge(args ResourceRevisionArgs) (*resourceRevision, error) {
	// Check fields match where set.
	mkErr := func(field string) error {
		return errors.Errorf("%s mismatch for revision %d", field, args.Revision)
	}
	checkStr := func(field, a, b string) error {
		if a != "" && b != "" && a != b {
			return errors.Trace(mkErr(field))
		}
		return nil
	}
	if err := checkStr("description", args.Description, r.Description_); err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkStr("type", args.Type, r.Type_); err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkStr("path", args.Path, r.Path_); err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkStr("origin", args.Origin, r.Origin_); err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkStr("fingerprint", args.FingerprintHex, r.FingerprintHex_); err != nil {
		return nil, errors.Trace(err)
	}
	if args.Size > 0 && r.Size_ > 0 && args.Size != r.Size_ {
		return nil, mkErr("size")
	}
	if !args.Timestamp.IsZero() && r.Timestamp_ != nil && args.Timestamp.UTC() != r.Timestamp() {
		return nil, mkErr("timestamp")
	}
	if err := checkStr("username", args.Username, r.Username_); err != nil {
		return nil, errors.Trace(err)
	}

	// Now merge.
	if args.Description != "" {
		r.Description_ = args.Description
	}
	if args.Type != "" {
		r.Type_ = args.Type
	}
	if args.Path != "" {
		r.Path_ = args.Path
	}
	if args.Origin != "" {
		r.Origin_ = args.Origin
	}
	if args.FingerprintHex != "" {
		r.FingerprintHex_ = args.FingerprintHex
	}
	if args.Size != 0 {
		r.Size_ = args.Size
	}
	if !args.Timestamp.IsZero() {
		r.Timestamp_ = timePtr(args.Timestamp)
	}
	if args.Username != "" {
		r.Username_ = args.Username
	}
	return r, nil
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
		"revisions":            schema.List(schema.StringMap(schema.Any())),
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
	revList := valid["revisions"].([]interface{})
	r := newResource(ResourceArgs{
		Name:               name,
		Revision:           int(valid["application-revision"].(int64)),
		CharmStoreRevision: int(valid["charmstore-revision"].(int64)),
	})
	for i, revSource := range revList {
		revision, err := importResourceRevisionV1(revSource)
		if err != nil {
			return nil, errors.Annotatef(err, "resource %s, revision index %d", name, i)
		}
		r.Revisions_ = append(r.Revisions_, revision)
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
