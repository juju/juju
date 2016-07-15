// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storetesting // import "gopkg.in/juju/charmstore.v5-unstable/internal/storetesting"

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable/testing"
	"gopkg.in/yaml.v2"

	"gopkg.in/juju/charmstore.v5-unstable/internal/blobstore"
)

// Charms holds the testing charm repository.
var Charms = testing.NewRepo("charm-repo", "quantal")

var _ charm.Bundle = (*Bundle)(nil)

// Bundle implements an in-memory charm.Bundle
// that can be archived.
//
// Note that because it implements charmstore.ArchiverTo,
// it can be used as an argument to charmstore.Store.AddBundleWithArchive.
type Bundle struct {
	blob     []byte
	blobHash string
	data     *charm.BundleData
	readMe   string
}

// Data implements charm.Bundle.Data.
func (b *Bundle) Data() *charm.BundleData {
	return b.data
}

// ReadMe implements charm.Bundle.ReadMe.
func (b *Bundle) ReadMe() string {
	return b.readMe
}

// ArchiveTo implements charmstore.ArchiverTo.
func (b *Bundle) ArchiveTo(w io.Writer) error {
	_, err := w.Write(b.blob)
	return err
}

// Bytes returns the contents of the bundle's archive.
func (b *Bundle) Bytes() []byte {
	return b.blob
}

// Size returns the size of the bundle's archive blob.
func (b *Bundle) Size() int64 {
	return int64(len(b.blob))
}

// NewBundle returns a bundle implementation
// that contains the given bundle data.
func NewBundle(data *charm.BundleData) *Bundle {
	dataYAML, err := yaml.Marshal(data)
	if err != nil {
		panic(err)
	}
	readMe := "boring"
	blob, hash := newBlob([]file{{
		name: "bundle.yaml",
		data: dataYAML,
	}, {
		name: "README.md",
		data: []byte(readMe),
	}})
	return &Bundle{
		blob:     blob,
		blobHash: hash,
		data:     data,
		readMe:   readMe,
	}
}

// Charm implements an in-memory charm.Charm that
// can be archived.
//
// Note that because it implements charmstore.ArchiverTo,
// it can be used as an argument to charmstore.Store.AddCharmWithArchive.
type Charm struct {
	blob     []byte
	blobHash string
	meta     *charm.Meta
}

var _ charm.Charm = (*Charm)(nil)

// NewCharm returns a charm implementation
// that contains the given charm metadata.
// All charm.Charm methods other than Meta will return empty values.
func NewCharm(meta *charm.Meta) *Charm {
	if meta == nil {
		meta = new(charm.Meta)
	}
	metaYAML, err := yaml.Marshal(meta)
	if err != nil {
		panic(err)
	}
	blob, hash := newBlob([]file{{
		name: "metadata.yaml",
		data: metaYAML,
	}, {
		name: "README.md",
		data: []byte("boring"),
	}})
	return &Charm{
		blob:     blob,
		blobHash: hash,
		meta:     meta,
	}
}

// Meta implements charm.Charm.Meta.
func (c *Charm) Meta() *charm.Meta {
	return c.meta
}

// Config implements charm.Charm.Config.
func (c *Charm) Config() *charm.Config {
	return charm.NewConfig()
}

// Metrics implements charm.Charm.Metrics.
func (c *Charm) Metrics() *charm.Metrics {
	return nil
}

// Actions implements charm.Charm.Actions.
func (c *Charm) Actions() *charm.Actions {
	return charm.NewActions()
}

// Revision implements charm.Charm.Revision.
func (c *Charm) Revision() int {
	return 0
}

// ArchiveTo implements charmstore.ArchiverTo.
func (c *Charm) ArchiveTo(w io.Writer) error {
	_, err := w.Write(c.blob)
	return err
}

// Bytes returns the contents of the charm's archive.
func (c *Charm) Bytes() []byte {
	return c.blob
}

// Size returns the size of the charm's archive blob.
func (c *Charm) Size() int64 {
	return int64(len(c.blob))
}

type file struct {
	name string
	data []byte
}

// newBlob returns a zip archive containing the given files.
func newBlob(files []file) ([]byte, string) {
	var blob bytes.Buffer
	zw := zip.NewWriter(&blob)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			panic(err)
		}
		if _, err := w.Write(f.data); err != nil {
			panic(err)
		}
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	h := blobstore.NewHash()
	h.Write(blob.Bytes())
	return blob.Bytes(), fmt.Sprintf("%x", h.Sum(nil))
}

// MetaWithSupportedSeries returns m with Series
// set to series. If m is nil, new(charm.Meta)
// will be used instead.
func MetaWithSupportedSeries(m *charm.Meta, series ...string) *charm.Meta {
	if m == nil {
		m = new(charm.Meta)
	}
	m.Series = series
	return m
}

// RelationMeta returns charm metadata for a charm
// with the given relations, where each relation
// is specified as a white-space-separated
// triple:
//	role name interface
// where role specifies the role of the interface
// (provides or requires), name holds the relation
// name and interface holds the interface relation type.
func RelationMeta(relations ...string) *charm.Meta {
	provides := make(map[string]charm.Relation)
	requires := make(map[string]charm.Relation)
	for _, rel := range relations {
		r, err := parseRelation(rel)
		if err != nil {
			panic(fmt.Errorf("bad relation %q", err))
		}
		if r.Role == charm.RoleProvider {
			provides[r.Name] = r
		} else {
			requires[r.Name] = r
		}
	}
	return &charm.Meta{
		Provides: provides,
		Requires: requires,
	}
}

func parseRelation(s string) (charm.Relation, error) {
	fields := strings.Fields(s)
	if len(fields) != 3 {
		return charm.Relation{}, errgo.Newf("wrong field count")
	}
	r := charm.Relation{
		Scope:     charm.ScopeGlobal,
		Name:      fields[1],
		Interface: fields[2],
	}
	switch fields[0] {
	case "provides":
		r.Role = charm.RoleProvider
	case "requires":
		r.Role = charm.RoleRequirer
	default:
		return charm.Relation{}, errgo.Newf("unknown role")
	}
	return r, nil
}

// MetaWithResources returns m with Resources set to a set of resources
// with the given names. If m is nil, new(charm.Meta) will be used
// instead.
//
// The path and description of the resources are derived from
// the resource name by adding a "-file" and a " description"
// suffix respectively.
func MetaWithResources(m *charm.Meta, resources ...string) *charm.Meta {
	if m == nil {
		m = new(charm.Meta)
	}
	m.Resources = make(map[string]resource.Meta)
	for _, name := range resources {
		m.Resources[name] = resource.Meta{
			Name:        name,
			Type:        resource.TypeFile,
			Path:        name + "-file",
			Description: name + " description",
		}
	}
	return m
}

// MetaWithCategories returns m with Categories set to categories. If m
// is nil, new(charm.Meta) will be used instead.
func MetaWithCategories(m *charm.Meta, categories ...string) *charm.Meta {
	if m == nil {
		m = new(charm.Meta)
	}
	m.Categories = categories
	return m
}

// MetaWithTags returns m with Tags set to tags. If m is nil,
// new(charm.Meta) will be used instead.
func MetaWithTags(m *charm.Meta, tags ...string) *charm.Meta {
	if m == nil {
		m = new(charm.Meta)
	}
	m.Tags = tags
	return m
}
