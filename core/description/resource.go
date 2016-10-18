// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import "time"

// Resource represents an application resource.
type Resource interface {
	Name() string
	Revision() int
	CharmStoreRevision() int
	AddRevision(ResourceRevisionArgs)
	Revisions() map[int]ResourceRevision
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
		Revisions_:          make(map[int]ResourceRevision),
	}
}

type resources struct {
	Version    int         `yaml:"version"`
	Resources_ []*resource `yaml:"resources"`
}

type resource struct {
	Name_               string                   `yaml:"name"`
	Revision_           int                      `yaml:"application-revision"`
	CharmStoreRevision_ int                      `yaml:"charmstore-revision"`
	CharmStorePolled_   time.Time                `yaml:"charmstore-polled"`
	Revisions_          map[int]ResourceRevision `yaml:"revisions"`
}

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
	rev := &resourceRevision{
		Revision_:     args.Revision,
		Type_:         args.Type,
		Path_:         args.Path,
		Description_:  args.Description,
		Origin_:       args.Origin,
		Fingerprint_:  args.Fingerprint,
		Size_:         args.Size,
		AddTimestamp_: args.AddTimestamp,
		Username_:     args.Username,
	}
	r.Revisions_[args.Revision] = rev
}

// Revisions implements Resource.
func (r *resource) Revisions() map[int]ResourceRevision {
	out := make(map[int]ResourceRevision)
	for k, v := range r.Revisions_ {
		out[k] = v
	}
	return out
}

type resourceRevision struct {
	Revision_     int
	Type_         string
	Path_         string
	Description_  string
	Origin_       string
	Fingerprint_  string
	Size_         int64
	AddTimestamp_ time.Time
	Username_     string
	StoragePath_  string
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
	return r.AddTimestamp_
}

// Username implements ResourceRevision.
func (r *resourceRevision) Username() string {
	return r.Username_
}
