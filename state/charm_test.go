// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type CharmSuite struct {
	ConnSuite
	curl *charm.URL
}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	added := s.AddTestingCharm(c, "dummy")
	s.curl = added.URL()
}

func (s *CharmSuite) TestCharm(c *gc.C) {
	dummy, err := s.State.Charm(s.curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL().String(), gc.Equals, s.curl.String())
	c.Assert(dummy.Revision(), gc.Equals, 1)
	c.Assert(dummy.StoragePath(), gc.Equals, "dummy-path")
	c.Assert(dummy.BundleSha256(), gc.Equals, "quantal-dummy-1-sha256")
	c.Assert(dummy.IsUploaded(), jc.IsTrue)
	meta := dummy.Meta()
	c.Assert(meta.Name, gc.Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], gc.Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the application.",
			Type:        "string",
		},
	)
	actions := dummy.Actions()
	c.Assert(actions, gc.NotNil)
	c.Assert(actions.ActionSpecs, gc.Not(gc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], gc.NotNil)
	c.Assert(actions.ActionSpecs["snapshot"].Params, gc.Not(gc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], gc.DeepEquals,
		charm.ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"type":        "object",
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2",
					},
				},
			},
		})
}

func (s *CharmSuite) TestCharmNotFound(c *gc.C) {
	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.State.Charm(curl)
	c.Assert(err, gc.ErrorMatches, `charm "local:anotherseries/dummy-1" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) dummyCharm(c *gc.C, curlOverride string) state.CharmInfo {
	info := state.CharmInfo{
		Charm:       testcharms.Repo.CharmDir("dummy"),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
	}
	if curlOverride != "" {
		info.ID = charm.MustParseURL(curlOverride)
	} else {
		info.ID = charm.MustParseURL(
			fmt.Sprintf("local:quantal/%s-%d", info.Charm.Meta().Name, info.Charm.Revision()),
		)
	}
	return info
}

func (s *CharmSuite) TestAddCharm(c *gc.C) {
	// Check that adding charms from scratch works correctly.
	info := s.dummyCharm(c, "")
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL().String(), gc.Equals, info.ID.String())

	doc := state.CharmDoc{}
	err = s.charms.FindId(state.DocID(s.State, info.ID.String())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, gc.DeepEquals, info.ID)
}

func (s *CharmSuite) TestAddCharmWithAuth(c *gc.C) {
	// Check that adding charms from scratch works correctly.
	info := s.dummyCharm(c, "")
	m, err := macaroon.New([]byte("rootkey"), "id", "loc")
	c.Assert(err, jc.ErrorIsNil)
	info.Macaroon = macaroon.Slice{m}
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	ms, err := dummy.Macaroon()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms, gc.DeepEquals, info.Macaroon)
}

func (s *CharmSuite) TestAddCharmUpdatesPlaceholder(c *gc.C) {
	// Check that adding charms updates any existing placeholder charm
	// with the same URL.
	ch := testcharms.Repo.CharmDir("dummy")

	// Add a placeholder charm.
	curl := charm.MustParseURL("cs:quantal/dummy-1")
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Add a deployed charm.
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
	}
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL().String(), gc.Equals, curl.String())

	// Charm doc has been updated.
	var docs []state.CharmDoc
	err = s.charms.FindId(state.DocID(s.State, curl.String())).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0].URL, gc.DeepEquals, curl)
	c.Assert(docs[0].StoragePath, gc.DeepEquals, info.StoragePath)

	// No more placeholder charm.
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) assertPendingCharmExists(c *gc.C, curl *charm.URL) {
	// Find charm directly and verify only the charm URL and
	// PendingUpload are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl.String())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, gc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, jc.IsTrue)
	c.Assert(doc.Placeholder, jc.IsFalse)
	c.Assert(doc.Meta, gc.IsNil)
	c.Assert(doc.Config, gc.IsNil)
	c.Assert(doc.StoragePath, gc.Equals, "")
	c.Assert(doc.BundleSha256, gc.Equals, "")

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) TestPrepareLocalCharmUpload(c *gc.C) {
	// First test the sanity checks.
	curl, err := s.State.PrepareLocalCharmUpload(charm.MustParseURL("local:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(curl, gc.IsNil)
	curl, err = s.State.PrepareLocalCharmUpload(charm.MustParseURL("cs:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with local schema, got .*")
	c.Assert(curl, gc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := charm.MustParseURL("local:quantal/missing-123")
	curl, err = s.State.PrepareLocalCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.DeepEquals, testCurl)
	s.assertPendingCharmExists(c, curl)

	// Try adding it again with the same revision and ensure it gets bumped.
	curl, err = s.State.PrepareLocalCharmUpload(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 124)

	// Also ensure the revision cannot decrease.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(42))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 125)

	// Check the given revision is respected.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(1234))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 1234)
}

func (s *CharmSuite) TestPrepareStoreCharmUpload(c *gc.C) {
	// First test the sanity checks.
	sch, err := s.State.PrepareStoreCharmUpload(charm.MustParseURL("cs:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(sch, gc.IsNil)
	sch, err = s.State.PrepareStoreCharmUpload(charm.MustParseURL("local:quantal/dummy"))
	c.Assert(err, gc.ErrorMatches, "expected charm URL with cs schema, got .*")
	c.Assert(sch, gc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := charm.MustParseURL("cs:quantal/missing-123")
	sch, err = s.State.PrepareStoreCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, testCurl)
	c.Assert(sch.IsUploaded(), jc.IsFalse)

	s.assertPendingCharmExists(c, sch.URL())

	// Try adding it again with the same revision and ensure we get the same document.
	schCopy, err := s.State.PrepareStoreCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch, jc.DeepEquals, schCopy)

	// Now add a charm and try again - we should get the same result
	// as with AddCharm.
	info := s.dummyCharm(c, "cs:precise/dummy-2")
	sch, err = s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	schCopy, err = s.State.PrepareStoreCharmUpload(info.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch, jc.DeepEquals, schCopy)
}

func (s *CharmSuite) TestUpdateUploadedCharm(c *gc.C) {
	info := s.dummyCharm(c, "")
	_, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Test with already uploaded and a missing charms.
	sch, err := s.State.UpdateUploadedCharm(info)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("charm %q already uploaded", info.ID))
	c.Assert(sch, gc.IsNil)
	info.ID = charm.MustParseURL("local:quantal/missing-1")
	info.SHA256 = "missing"
	sch, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(sch, gc.IsNil)

	// Test with with an uploaded local charm.
	_, err = s.State.PrepareLocalCharmUpload(info.ID)
	c.Assert(err, jc.ErrorIsNil)

	m, err := macaroon.New([]byte("rootkey"), "id", "loc")
	c.Assert(err, jc.ErrorIsNil)
	info.Macaroon = macaroon.Slice{m}
	c.Assert(err, jc.ErrorIsNil)
	sch, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, info.ID)
	c.Assert(sch.Revision(), gc.Equals, info.ID.Revision)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.Meta(), gc.DeepEquals, info.Charm.Meta())
	c.Assert(sch.Config(), gc.DeepEquals, info.Charm.Config())
	c.Assert(sch.StoragePath(), gc.DeepEquals, info.StoragePath)
	c.Assert(sch.BundleSha256(), gc.Equals, "missing")
	ms, err := sch.Macaroon()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms, gc.DeepEquals, info.Macaroon)
}

func (s *CharmSuite) TestUpdateUploadedCharmEscapesSpecialCharsInConfig(c *gc.C) {
	// Make sure when we have mongodb special characters like "$" and
	// "." in the name of any charm config option, we do proper
	// escaping before storing them and unescaping after loading. See
	// also http://pad.lv/1308146.

	// Clone the dummy charm and change the config.
	configWithProblematicKeys := []byte(`
options:
  $bad.key: {default: bad, description: bad, type: string}
  not.ok.key: {description: not ok, type: int}
  valid-key: {description: all good, type: boolean}
  still$bad.: {description: not good, type: float}
  $.$: {description: awful, type: string}
  ...: {description: oh boy, type: int}
  just$: {description: no no, type: float}
`[1:])
	chDir := testcharms.Repo.ClonedDirPath(c.MkDir(), "dummy")
	err := utils.AtomicWriteFile(
		filepath.Join(chDir, "config.yaml"),
		configWithProblematicKeys,
		0666,
	)
	c.Assert(err, jc.ErrorIsNil)
	ch, err := charm.ReadCharmDir(chDir)
	c.Assert(err, jc.ErrorIsNil)
	missingCurl := charm.MustParseURL("local:quantal/missing-1")
	storagePath := "dummy-1"

	preparedCurl, err := s.State.PrepareLocalCharmUpload(missingCurl)
	c.Assert(err, jc.ErrorIsNil)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          preparedCurl,
		StoragePath: "dummy-1",
		SHA256:      "missing",
	}
	sch, err := s.State.UpdateUploadedCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, missingCurl)
	c.Assert(sch.Revision(), gc.Equals, missingCurl.Revision)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(sch.Config(), gc.DeepEquals, ch.Config())
	c.Assert(sch.StoragePath(), gc.DeepEquals, storagePath)
	c.Assert(sch.BundleSha256(), gc.Equals, "missing")
}

func (s *CharmSuite) assertPlaceholderCharmExists(c *gc.C, curl *charm.URL) {
	// Find charm directly and verify only the charm URL and
	// Placeholder are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl.String())).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doc.URL, gc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, jc.IsFalse)
	c.Assert(doc.Placeholder, jc.IsTrue)
	c.Assert(doc.Meta, gc.IsNil)
	c.Assert(doc.Config, gc.IsNil)
	c.Assert(doc.StoragePath, gc.Equals, "")
	c.Assert(doc.BundleSha256, gc.Equals, "")

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CharmSuite) TestLatestPlaceholderCharm(c *gc.C) {
	// Add a deployed charm
	info := s.dummyCharm(c, "cs:quantal/dummy-1")
	_, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Deployed charm not found.
	_, err = s.State.LatestPlaceholderCharm(info.ID)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Add a charm reference
	curl2 := charm.MustParseURL("cs:quantal/dummy-2")
	err = s.State.AddStoreCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2)

	// Use a URL with an arbitrary rev to search.
	curl := charm.MustParseURL("cs:quantal/dummy-23")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pending.URL(), gc.DeepEquals, curl2)
	c.Assert(pending.IsPlaceholder(), jc.IsTrue)
	c.Assert(pending.Meta(), gc.IsNil)
	c.Assert(pending.Config(), gc.IsNil)
	c.Assert(pending.StoragePath(), gc.Equals, "")
	c.Assert(pending.BundleSha256(), gc.Equals, "")
}

func (s *CharmSuite) TestAddStoreCharmPlaceholderErrors(c *gc.C) {
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, gc.ErrorMatches, "expected charm URL with cs schema, got .*")

	curl = charm.MustParseURL("cs:quantal/dummy")
	err = s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
}

func (s *CharmSuite) TestAddStoreCharmPlaceholder(c *gc.C) {
	curl := charm.MustParseURL("cs:quantal/dummy-1")
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl)

	// Add the same one again, should be a no-op
	err = s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl)
}

func (s *CharmSuite) assertAddStoreCharmPlaceholder(c *gc.C) (*charm.URL, *charm.URL, *state.Charm) {
	// Add a deployed charm
	info := s.dummyCharm(c, "cs:quantal/dummy-1")
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Add a charm placeholder
	curl2 := charm.MustParseURL("cs:quantal/dummy-2")
	err = s.State.AddStoreCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2)

	// Deployed charm is still there.
	existing, err := s.State.Charm(info.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existing, jc.DeepEquals, dummy)

	return info.ID, curl2, dummy
}

func (s *CharmSuite) TestAddStoreCharmPlaceholderLeavesDeployedCharmsAlone(c *gc.C) {
	s.assertAddStoreCharmPlaceholder(c)
}

func (s *CharmSuite) TestAddStoreCharmPlaceholderDeletesOlder(c *gc.C) {
	curl, curlOldRef, dummy := s.assertAddStoreCharmPlaceholder(c)

	// Add a new charm placeholder
	curl3 := charm.MustParseURL("cs:quantal/dummy-3")
	err := s.State.AddStoreCharmPlaceholder(curl3)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl3)

	// Deployed charm is still there.
	existing, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existing, jc.DeepEquals, dummy)

	// Older charm placeholder is gone.
	doc := state.CharmDoc{}
	err = s.charms.FindId(curlOldRef).One(&doc)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *CharmSuite) TestAllCharms(c *gc.C) {
	// Add a deployed charm
	info := s.dummyCharm(c, "cs:quantal/dummy-1")
	sch, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Add a charm reference
	curl2 := charm.MustParseURL("cs:quantal/dummy-2")
	err = s.State.AddStoreCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)

	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charms, gc.HasLen, 3)

	c.Assert(charms[0].URL().String(), gc.Equals, "local:quantal/quantal-dummy-1")
	c.Assert(charms[1], gc.DeepEquals, sch)
	c.Assert(charms[2].URL(), gc.DeepEquals, curl2)
}

type CharmTestHelperSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CharmTestHelperSuite{})

func assertCustomCharm(c *gc.C, ch *state.Charm, series string, meta *charm.Meta, config *charm.Config, metrics *charm.Metrics, revision int) {
	// Check Charm interface method results.
	c.Assert(ch.Meta(), gc.DeepEquals, meta)
	c.Assert(ch.Config(), gc.DeepEquals, config)
	c.Assert(ch.Metrics(), gc.DeepEquals, metrics)
	c.Assert(ch.Revision(), gc.DeepEquals, revision)

	// Test URL matches charm and expected series.
	url := ch.URL()
	c.Assert(url.Series, gc.Equals, series)
	c.Assert(url.Revision, gc.Equals, ch.Revision())

	// Ignore the StoragePath and BundleSHA256 methods, they're irrelevant.
}

func assertStandardCharm(c *gc.C, ch *state.Charm, series string) {
	chd := testcharms.Repo.CharmDir(ch.Meta().Name)
	assertCustomCharm(c, ch, series, chd.Meta(), chd.Config(), chd.Metrics(), chd.Revision())
}

func forEachStandardCharm(c *gc.C, f func(name string)) {
	for _, name := range []string{
		"logging", "mysql", "riak", "wordpress",
	} {
		c.Logf("checking %s", name)
		f(name)
	}
}

func (s *CharmTestHelperSuite) TestSimple(c *gc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		config := chd.Config()
		metrics := chd.Metrics()
		revision := chd.Revision()

		ch := s.AddTestingCharm(c, name)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, revision)

		ch = s.AddSeriesCharm(c, name, "anotherseries")
		assertCustomCharm(c, ch, "anotherseries", meta, config, metrics, revision)
	})
}

var configYaml = `
options:
  working:
    description: when set to false, prevents service from functioning correctly
    default: true
    type: boolean
`

func (s *CharmTestHelperSuite) TestConfigCharm(c *gc.C) {
	config, err := charm.ReadConfig(bytes.NewBuffer([]byte(configYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		metrics := chd.Metrics()

		ch := s.AddConfigCharm(c, name, configYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

var actionsYaml = `
actions:
   dump:
      description: Dump the database to STDOUT.
      params:
         redirect-file:
            description: Redirect to a log file.
            type: string
`

func (s *CharmTestHelperSuite) TestActionsCharm(c *gc.C) {
	actions, err := charm.ReadActionsYaml(bytes.NewBuffer([]byte(actionsYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		ch := s.AddActionsCharm(c, name, actionsYaml, 123)
		c.Assert(ch.Actions(), gc.DeepEquals, actions)
	})
}

var metricsYaml = `
metrics:
  blips:
    description: A custom metric.
    type: gauge
`

func (s *CharmTestHelperSuite) TestMetricsCharm(c *gc.C) {
	metrics, err := charm.ReadMetrics(bytes.NewBuffer([]byte(metricsYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		config := chd.Config()

		ch := s.AddMetricsCharm(c, name, metricsYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

var metaYamlSnippet = `
summary: blah
description: blah blah
`

func (s *CharmTestHelperSuite) TestMetaCharm(c *gc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		config := chd.Config()
		metrics := chd.Metrics()
		metaYaml := "name: " + name + metaYamlSnippet
		meta, err := charm.ReadMeta(bytes.NewBuffer([]byte(metaYaml)))
		c.Assert(err, jc.ErrorIsNil)

		ch := s.AddMetaCharm(c, name, metaYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

func (s *CharmTestHelperSuite) TestTestingCharm(c *gc.C) {
	added := s.AddTestingCharm(c, "metered")
	c.Assert(added.Metrics(), gc.NotNil)

	chd := testcharms.Repo.CharmDir("metered")
	c.Assert(chd.Metrics(), gc.DeepEquals, added.Metrics())
}
