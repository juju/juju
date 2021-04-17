// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type computedSeriesSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&computedSeriesSuite{})

func (s *computedSeriesSuite) TestComputedSeriesLegacy(c *gc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
 name: a
 summary: b
 description: c
 series:
   - bionic
 `))
	c.Assert(err, gc.IsNil)
	series, err := ComputedSeries(charmMeta{meta: meta, manifest: &charm.Manifest{}})
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"bionic"})
}

func (s *computedSeriesSuite) TestComputedSeriesNilManifest(c *gc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
 name: a
 summary: b
 description: c
 series:
   - bionic
 `))
	c.Assert(err, gc.IsNil)
	series, err := ComputedSeries(charmMeta{meta: meta})
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"bionic"})
}

func (s *computedSeriesSuite) TestComputedSeries(c *gc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
 name: a
 summary: b
 description: c
 `))
	c.Assert(err, gc.IsNil)
	manifest, err := charm.ReadManifest(strings.NewReader(`
 bases:
   - name: ubuntu
     channel: "18.04"
   - name: ubuntu
     channel: "20.04"
 `))
	c.Assert(err, gc.IsNil)
	series, err := ComputedSeries(charmMeta{meta: meta, manifest: manifest})
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"bionic", "focal"})
}

func (s *computedSeriesSuite) TestComputedSeriesKubernetes(c *gc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
 name: a
 summary: b
 description: c
 containers:
   redis:
     resource: redis-container-resource
 resources:
     redis-container-resource:
       name: redis-container
       type: oci-image
 `))
	c.Assert(err, gc.IsNil)
	manifest, err := charm.ReadManifest(strings.NewReader(`
 bases:
   - name: ubuntu
     channel: "18.04"
 `))
	c.Assert(err, gc.IsNil)
	series, err := ComputedSeries(charmMeta{meta: meta, manifest: manifest})
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"kubernetes"})
}

func (s *computedSeriesSuite) TestComputedSeriesError(c *gc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
 name: a
 summary: b
 description: c
 `))
	c.Assert(err, gc.IsNil)
	manifest, err := charm.ReadManifest(strings.NewReader(`
 bases:
   - name: ubuntu
     channel: "18.04"
   - name: ubuntu
     channel: "testme"
 `))
	c.Assert(err, gc.IsNil)
	_, err = ComputedSeries(charmMeta{meta: meta, manifest: manifest})
	c.Assert(err, gc.ErrorMatches, `unknown series for version: "testme"`)
}

type charmMeta struct {
	meta     *charm.Meta
	manifest *charm.Manifest
}

func (cm charmMeta) Manifest() *charm.Manifest {
	return cm.manifest
}

func (cm charmMeta) Meta() *charm.Meta {
	return cm.meta
}
