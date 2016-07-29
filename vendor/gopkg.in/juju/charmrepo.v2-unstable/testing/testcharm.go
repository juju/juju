// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing // import "gopkg.in/juju/charmrepo.v2-unstable/testing"

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/yaml.v2"
)

// Charm holds a charm for testing. It does not
// have a representation on disk by default, but
// can be written to disk using Archive and its ExpandTo
// method. It implements the charm.Charm interface.
//
// All methods on Charm may be called concurrently.
type Charm struct {
	meta     *charm.Meta
	config   *charm.Config
	actions  *charm.Actions
	metrics  *charm.Metrics
	revision int

	files filetesting.Entries

	makeArchiveOnce sync.Once
	archiveBytes    []byte
	archive         *charm.CharmArchive
}

// CharmSpec holds the specification for a charm. The fields
// hold data in YAML format.
type CharmSpec struct {
	// Meta holds the contents of metadata.yaml.
	Meta string

	// Config holds the contents of config.yaml.
	Config string

	// Actions holds the contents of actions.yaml.
	Actions string

	// Metrics holds the contents of metrics.yaml.
	Metrics string

	// Files holds any additional files that should be
	// added to the charm. If this is nil, a minimal set
	// of files will be added to ensure the charm is readable.
	Files []filetesting.Entry

	// Revision specifies the revision of the charm.
	Revision int
}

type file struct {
	path string
	data []byte
	perm os.FileMode
}

// NewCharm returns a charm following the given specification.
func NewCharm(c *gc.C, spec CharmSpec) *Charm {
	return newCharm(spec)
}

// newCharm is the internal version of NewCharm that
// doesn't take a *gc.C so it can be used in NewCharmWithMeta.
func newCharm(spec CharmSpec) *Charm {
	ch := &Charm{
		revision: spec.Revision,
	}
	var err error
	ch.meta, err = charm.ReadMeta(strings.NewReader(spec.Meta))
	if err != nil {
		panic(err)
	}

	ch.files = append(ch.files, filetesting.File{
		Path: "metadata.yaml",
		Data: spec.Meta,
		Perm: 0644,
	})

	if spec.Config != "" {
		ch.config, err = charm.ReadConfig(strings.NewReader(spec.Config))
		if err != nil {
			panic(err)
		}
		ch.files = append(ch.files, filetesting.File{
			Path: "config.yaml",
			Data: spec.Config,
			Perm: 0644,
		})
	}
	if spec.Actions != "" {
		ch.actions, err = charm.ReadActionsYaml(strings.NewReader(spec.Actions))
		if err != nil {
			panic(err)
		}
		ch.files = append(ch.files, filetesting.File{
			Path: "actions.yaml",
			Data: spec.Actions,
			Perm: 0644,
		})
	}
	if spec.Metrics != "" {
		ch.metrics, err = charm.ReadMetrics(strings.NewReader(spec.Metrics))
		if err != nil {
			panic(err)
		}
		ch.files = append(ch.files, filetesting.File{
			Path: "metrics.yaml",
			Data: spec.Metrics,
			Perm: 0644,
		})
	}
	if spec.Files == nil {
		ch.files = append(ch.files, filetesting.File{
			Path: "hooks/install",
			Data: "#!/bin/sh\n",
			Perm: 0755,
		}, filetesting.File{
			Path: "hooks/start",
			Data: "#!/bin/sh\n",
			Perm: 0755,
		})
	} else {
		ch.files = append(ch.files, spec.Files...)
		// Check for duplicates.
		names := make(map[string]bool)
		for _, f := range ch.files {
			name := path.Clean(f.GetPath())
			if names[name] {
				panic(fmt.Errorf("duplicate file entry %q", f.GetPath()))
			}
			names[name] = true
		}
	}
	return ch
}

// NewCharmMeta returns a charm with the given metadata.
// It doesn't take a *gc.C, so it can be used at init time,
// for example in table-driven tests.
func NewCharmMeta(meta *charm.Meta) *Charm {
	if meta == nil {
		meta = new(charm.Meta)
	}
	metaYAML, err := yaml.Marshal(meta)
	if err != nil {
		panic(err)
	}
	return newCharm(CharmSpec{
		Meta: string(metaYAML),
	})
}

// Meta implements charm.Charm.Meta.
func (ch *Charm) Meta() *charm.Meta {
	return ch.meta
}

// Config implements charm.Charm.Config.
func (ch *Charm) Config() *charm.Config {
	if ch.config == nil {
		return &charm.Config{
			Options: map[string]charm.Option{},
		}
	}
	return ch.config
}

// Metrics implements charm.Charm.Metrics.
func (ch *Charm) Metrics() *charm.Metrics {
	return ch.metrics
}

// Actions implements charm.Charm.Actions.
func (ch *Charm) Actions() *charm.Actions {
	if ch.actions == nil {
		return &charm.Actions{}
	}
	return ch.actions
}

// Revision implements charm.Charm.Revision.
func (ch *Charm) Revision() int {
	return ch.revision
}

// Archive returns a charm archive holding the charm.
func (ch *Charm) Archive() *charm.CharmArchive {
	ch.makeArchiveOnce.Do(ch.makeArchive)
	return ch.archive
}

// ArchiveBytes returns the contents of the charm archive
// holding the charm.
func (ch *Charm) ArchiveBytes() []byte {
	ch.makeArchiveOnce.Do(ch.makeArchive)
	return ch.archiveBytes
}

// ArchiveTo implements ArchiveTo as implemented
// by *charm.Dir, enabling the charm to be used in some APIs
// that check for that method.
func (c *Charm) ArchiveTo(w io.Writer) error {
	_, err := w.Write(c.ArchiveBytes())
	return err
}

// Size returns the size of the charm's archive blob.
func (c *Charm) Size() int64 {
	return int64(len(c.ArchiveBytes()))
}

func (ch *Charm) makeArchive() {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, f := range ch.files {
		addZipEntry(zw, f)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	// ReadCharmArchiveFromReader requires a ReaderAt, so make one.
	r := bytes.NewReader(buf.Bytes())

	// Actually make the charm archive.
	archive, err := charm.ReadCharmArchiveFromReader(r, int64(buf.Len()))
	if err != nil {
		panic(err)
	}
	ch.archiveBytes = buf.Bytes()
	ch.archive = archive
	ch.archive.SetRevision(ch.revision)
}

func addZipEntry(zw *zip.Writer, f filetesting.Entry) {
	h := &zip.FileHeader{
		Name: f.GetPath(),
		// Don't bother compressing - the contents are so small that
		// it will just slow things down for no particular benefit.
		Method: zip.Store,
	}
	contents := ""
	switch f := f.(type) {
	case filetesting.Dir:
		h.SetMode(os.ModeDir | 0755)
	case filetesting.File:
		h.SetMode(f.Perm)
		contents = f.Data
	case filetesting.Symlink:
		h.SetMode(os.ModeSymlink | 0777)
		contents = f.Link
	}
	w, err := zw.CreateHeader(h)
	if err != nil {
		panic(err)
	}
	if contents != "" {
		if _, err := w.Write([]byte(contents)); err != nil {
			panic(err)
		}
	}
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

// MetaWithRelations returns m with Provides and Requires set
// to the given relations, where each relation
// is specified as a white-space-separated
// triple:
//	role name interface
// where role specifies the role of the interface
// ("provides" or "requires"), name holds the relation
// name and interface holds the interface relation type.
//
// If m is nil, new(charm.Meta) will be used instead.
func MetaWithRelations(m *charm.Meta, relations ...string) *charm.Meta {
	if m == nil {
		m = new(charm.Meta)
	}
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
	m.Provides = provides
	m.Requires = requires
	return m
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
