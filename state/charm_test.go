// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

// TODO (hml) lxd-profile
// Go back and add additional tests here

type CharmSuite struct {
	ConnSuite
	charm *state.Charm
	curl  string
}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	s.curl = s.charm.URL()
}

func (s *CharmSuite) destroy(c *gc.C) {
	err := s.charm.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CharmSuite) remove(c *gc.C) {
	s.destroy(c)
	err := s.charm.Remove(context.Background(), state.NewObjectStore(c, s.State))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CharmSuite) checkRemoved(c *gc.C) {
	_, err := s.State.Charm(s.curl)
	c.Check(err, gc.ErrorMatches, `charm ".*" not found`)
	c.Check(err, jc.ErrorIs, errors.NotFound)

	// Ensure the document is actually gone.
	coll, closer := state.GetCollection(s.State, "charms")
	defer closer()
	count, err := coll.FindId(s.curl).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)
}

func (s *CharmSuite) TestAliveCharm(c *gc.C) {
	s.testCharm(c)
}

func (s *CharmSuite) TestDyingCharm(c *gc.C) {
	s.destroy(c)
	s.testCharm(c)
}

func (s *CharmSuite) testCharm(c *gc.C) {
	dummy, err := s.State.Charm(s.curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL(), gc.Equals, s.curl)
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

func (s *CharmSuite) TestCharmFromSha256(c *gc.C) {
	ch, err := s.State.Charm(s.curl)
	c.Assert(err, jc.ErrorIsNil)

	dummy, err := s.State.CharmFromSha256(ch.BundleSha256()[0:7])

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL(), gc.Equals, s.curl)
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

func (s *CharmSuite) TestRemovedCharmNotFound(c *gc.C) {
	s.remove(c)
	s.checkRemoved(c)
}

func (s *CharmSuite) TestRemovedCharmNotListed(c *gc.C) {
	s.remove(c)
	charms, err := s.State.AllCharms()
	c.Check(err, jc.ErrorIsNil)
	c.Check(charms, gc.HasLen, 0)
}

func (s *CharmSuite) TestRemoveWithoutDestroy(c *gc.C) {
	err := s.charm.Remove(context.Background(), state.NewObjectStore(c, s.State))
	c.Assert(err, gc.ErrorMatches, "still alive")
}

func (s *CharmSuite) TestCharmNotFound(c *gc.C) {
	curl := "local:anotherseries/dummy-1"
	_, err := s.State.Charm(curl)
	c.Assert(err, gc.ErrorMatches, `charm "local:anotherseries/dummy-1" not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CharmSuite) TestCharmFromSha256NotFound(c *gc.C) {
	_, err := s.State.CharmFromSha256("abcd0123")
	c.Assert(err, gc.ErrorMatches, `charm with sha256 "abcd0123" not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CharmSuite) dummyCharm(c *gc.C, curlOverride string) state.CharmInfo {
	info := state.CharmInfo{
		Charm:       testcharms.Repo.CharmDir("dummy"),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
		Version:     "dummy-146-g725cfd3-dirty",
	}
	if curlOverride != "" {
		info.ID = curlOverride
	} else {
		info.ID = fmt.Sprintf("local:quantal/%s-%d", info.Charm.Meta().Name, info.Charm.Revision())
	}
	return info
}

func (s *CharmSuite) TestRemoveDeletesStorage(c *gc.C) {
	// We normally don't actually set up charm storage in state
	// tests, but we need it here.
	stor := state.NewObjectStore(c, s.State)
	path := s.charm.StoragePath()
	err := stor.Put(context.Background(), path, strings.NewReader("abc"), 3)
	c.Assert(err, jc.ErrorIsNil)

	s.destroy(c)
	closer, _, err := stor.Get(context.Background(), path)
	c.Assert(err, jc.ErrorIsNil)
	closer.Close()

	s.remove(c)
	_, _, err = stor.Get(context.Background(), path)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CharmSuite) TestReferenceDyingCharm(c *gc.C) {

	s.destroy(c)

	args := state.AddApplicationArgs{
		Name:  "blah",
		Charm: s.charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}
	_, err := s.State.AddApplication(args, state.NewObjectStore(c, s.State))
	c.Check(err, gc.ErrorMatches, `cannot add application "blah": charm: not found or not alive`)
}

func (s *CharmSuite) TestReferenceDyingCharmRace(c *gc.C) {

	defer state.SetBeforeHooks(c, s.State, func() {
		s.destroy(c)
	}).Check()

	args := state.AddApplicationArgs{
		Name:  "blah",
		Charm: s.charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "22.04/stable",
		}},
	}
	_, err := s.State.AddApplication(args, state.NewObjectStore(c, s.State))
	c.Check(err, gc.ErrorMatches, `cannot add application "blah": charm: not found or not alive`)
}

func (s *CharmSuite) TestDestroyReferencedCharm(c *gc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})

	err := s.charm.Destroy()
	c.Check(err, gc.ErrorMatches, "charm in use")
}

func (s *CharmSuite) TestDestroyReferencedCharmRace(c *gc.C) {

	defer state.SetBeforeHooks(c, s.State, func() {
		s.Factory.MakeApplication(c, &factory.ApplicationParams{
			Charm: s.charm,
		})
	}).Check()

	err := s.charm.Destroy()
	c.Check(err, gc.ErrorMatches, "charm in use")
}

func (s *CharmSuite) TestDestroyUnreferencedCharm(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})
	err := app.Destroy(state.NewObjectStore(c, s.State))
	c.Assert(err, jc.ErrorIsNil)

	err = s.charm.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CharmSuite) TestDestroyUnitReferencedCharm(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
		SetCharmURL: true,
	})

	// set app charm to something different
	info := s.dummyCharm(c, "ch:quantal/dummy-2")
	newCh, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	err = app.SetCharm(state.SetCharmConfig{Charm: newCh, CharmOrigin: defaultCharmOrigin(newCh.URL())}, state.NewObjectStore(c, s.State))
	c.Assert(err, jc.ErrorIsNil)

	// unit should still reference original charm until updated
	err = s.charm.Destroy()
	c.Assert(err, gc.ErrorMatches, "charm in use")
	err = unit.SetCharmURL(info.ID)
	c.Assert(err, jc.ErrorIsNil)
	err = s.charm.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CharmSuite) TestDestroyFinalUnitReference(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.charm,
	})
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("calling app.Destroy()")
	c.Assert(app.Destroy(state.NewObjectStore(c, s.State)), jc.ErrorIsNil)
	removeUnit(c, s.State, unit)

	assertCleanupCount(c, s.State, 2)
	s.checkRemoved(c)
}

func (s *CharmSuite) TestAddCharm(c *gc.C) {
	// Check that adding charms from scratch works correctly.
	info := s.dummyCharm(c, "")
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL(), gc.Equals, info.ID)

	doc := state.CharmDoc{}
	err = s.charms.FindId(state.DocID(s.State, info.ID)).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(*doc.URL, gc.DeepEquals, info.ID)

	expVersion := "dummy-146-g725cfd3-dirty"
	c.Assert(doc.CharmVersion, gc.Equals, expVersion)
}

func (s *CharmSuite) TestAddCharmUpdatesPlaceholder(c *gc.C) {
	// Check that adding charms updates any existing placeholder charm
	// with the same URL.
	ch := testcharms.Repo.CharmDir("dummy")

	// Add a placeholder charm.
	curl := charm.MustParseURL("ch:quantal/dummy-1")
	err := s.State.AddCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)

	// Add a deployed charm.
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl.String(),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
	}
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL(), gc.Equals, curl.String())

	// Charm doc has been updated.
	var docs []state.CharmDoc
	err = s.charms.FindId(state.DocID(s.State, curl.String())).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(*docs[0].URL, gc.DeepEquals, curl.String())
	c.Assert(docs[0].StoragePath, gc.DeepEquals, info.StoragePath)

	// No more placeholder charm.
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CharmSuite) assertPendingCharmExists(c *gc.C, curl string) {
	// Find charm directly and verify only the charm URL and
	// PendingUpload are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl)).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", doc)
	c.Assert(*doc.URL, gc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, jc.IsTrue)
	c.Assert(doc.Placeholder, jc.IsFalse)
	c.Assert(doc.Meta, gc.IsNil)
	c.Assert(doc.Config, gc.IsNil)
	c.Assert(doc.StoragePath, gc.Equals, "")
	c.Assert(doc.BundleSha256, gc.Equals, "")
}

func (s *CharmSuite) TestAddCharmWithInvalidMetaData(c *gc.C) {
	check := func(munge func(meta *charm.Meta)) {
		info := s.dummyCharm(c, "")
		meta := info.Charm.Meta()
		munge(meta)
		_, err := s.State.AddCharm(info)
		c.Assert(err, gc.ErrorMatches, `invalid charm data: "\$foo" is not a valid field name`)
	}

	check(func(meta *charm.Meta) {
		meta.Provides = map[string]charm.Relation{"$foo": {}}
	})
	check(func(meta *charm.Meta) {
		meta.Requires = map[string]charm.Relation{"$foo": {}}
	})
	check(func(meta *charm.Meta) {
		meta.Peers = map[string]charm.Relation{"$foo": {}}
	})
}

func (s *CharmSuite) TestPrepareLocalCharmUpload(c *gc.C) {
	// First test the sanity checks.
	curl, err := s.State.PrepareLocalCharmUpload("local:quantal/dummy")
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(curl, gc.IsNil)
	curl, err = s.State.PrepareLocalCharmUpload("ch:quantal/dummy")
	c.Assert(err, gc.ErrorMatches, "expected charm URL with local schema, got .*")
	c.Assert(curl, gc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := "local:quantal/missing-123"
	curl, err = s.State.PrepareLocalCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.String(), gc.Equals, testCurl)
	s.assertPendingCharmExists(c, curl.String())

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl.String())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Try adding it again with the same revision and ensure it gets bumped.
	curl, err = s.State.PrepareLocalCharmUpload(curl.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 124)

	// Also ensure the revision cannot decrease.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(42).String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 125)

	// Check the given revision is respected.
	curl, err = s.State.PrepareLocalCharmUpload(curl.WithRevision(1234).String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, 1234)
}

func (s *CharmSuite) TestPrepareLocalCharmUploadRemoved(c *gc.C) {
	// Remove the fixture charm and try to re-add it; it gets a new
	// revision.
	s.remove(c)
	curl, err := s.State.PrepareLocalCharmUpload(s.curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl.Revision, gc.Equals, charm.MustParseURL(s.curl).Revision+1)
}

func (s *CharmSuite) TestPrepareCharmUpload(c *gc.C) {
	// First test the sanity checks.
	sch, err := s.State.PrepareCharmUpload("ch:quantal/dummy")
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
	c.Assert(sch, gc.IsNil)
	sch, err = s.State.PrepareCharmUpload("local:quantal/dummy")
	c.Assert(err, gc.ErrorMatches, "expected charm URL with a valid schema, got .*")
	c.Assert(sch, gc.IsNil)

	// No charm in state, so the call should respect given revision.
	testCurl := "ch:quantal/missing-123"
	sch, err = s.State.PrepareCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, testCurl)
	c.Assert(sch.IsUploaded(), jc.IsFalse)

	s.assertPendingCharmExists(c, sch.URL())
	// Make sure we can find it with st.Charm().
	found, err := s.State.Charm(sch.URL())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.URL(), gc.Equals, sch.URL())

	// Try adding it again with the same revision and ensure we get the same document.
	schCopy, err := s.State.PrepareCharmUpload(testCurl)
	c.Assert(err, jc.ErrorIsNil)
	// URL is required to set the charmURL, so the test will succeed.
	_ = schCopy.URL()
	c.Assert(sch, jc.DeepEquals, schCopy)

	// Now add a charm and try again - we should get the same result
	// as with AddCharm.
	info := s.dummyCharm(c, "ch:precise/dummy-2")
	sch, err = s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	schCopy, err = s.State.PrepareCharmUpload(info.ID)
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
	info.ID = "local:quantal/missing-1"
	info.SHA256 = "missing"
	sch, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(sch, gc.IsNil)

	// Test with an uploaded local charm.
	_, err = s.State.PrepareLocalCharmUpload(info.ID)
	c.Assert(err, jc.ErrorIsNil)

	sch, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, info.ID)
	c.Assert(sch.Revision(), gc.Equals, charm.MustParseURL(info.ID).Revision)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.Meta(), gc.DeepEquals, info.Charm.Meta())
	c.Assert(sch.Config(), gc.DeepEquals, info.Charm.Config())
	c.Assert(sch.StoragePath(), gc.DeepEquals, info.StoragePath)
	c.Assert(sch.BundleSha256(), gc.Equals, "missing")
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
	missingCurl := "local:quantal/missing-1"
	storagePath := "dummy-1"

	preparedCurl, err := s.State.PrepareLocalCharmUpload(missingCurl)
	c.Assert(err, jc.ErrorIsNil)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          preparedCurl.String(),
		StoragePath: "dummy-1",
		SHA256:      "missing",
	}
	sch, err := s.State.UpdateUploadedCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), gc.DeepEquals, missingCurl)
	c.Assert(sch.Revision(), gc.Equals, charm.MustParseURL(missingCurl).Revision)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(sch.Config(), gc.DeepEquals, ch.Config())
	c.Assert(sch.StoragePath(), gc.DeepEquals, storagePath)
	c.Assert(sch.BundleSha256(), gc.Equals, "missing")
}

func (s *CharmSuite) assertPlaceholderCharmExists(c *gc.C, curl string) {
	// Find charm directly and verify only the charm URL and
	// Placeholder are set.
	doc := state.CharmDoc{}
	err := s.charms.FindId(state.DocID(s.State, curl)).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*doc.URL, gc.DeepEquals, curl)
	c.Assert(doc.PendingUpload, jc.IsFalse)
	c.Assert(doc.Placeholder, jc.IsTrue)
	c.Assert(doc.Meta, gc.IsNil)
	c.Assert(doc.Config, gc.IsNil)
	c.Assert(doc.StoragePath, gc.Equals, "")
	c.Assert(doc.BundleSha256, gc.Equals, "")

	// Make sure we can't find it with st.Charm().
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *CharmSuite) TestUpdateUploadedCharmRejectsInvalidMetadata(c *gc.C) {
	info := s.dummyCharm(c, "")
	_, err := s.State.PrepareLocalCharmUpload(info.ID)
	c.Assert(err, jc.ErrorIsNil)

	meta := info.Charm.Meta()
	meta.Provides = map[string]charm.Relation{
		"foo.bar": {},
	}
	_, err = s.State.UpdateUploadedCharm(info)
	c.Assert(err, gc.ErrorMatches, `invalid charm data: "foo.bar" is not a valid field name`)
}

func (s *CharmSuite) TestLatestPlaceholderCharm(c *gc.C) {
	// Add a deployed charm
	info := s.dummyCharm(c, "ch:quantal/dummy-1")
	_, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Deployed charm not found.
	_, err = s.State.LatestPlaceholderCharm(charm.MustParseURL(info.ID))
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Add a charm reference
	curl2 := charm.MustParseURL("ch:quantal/dummy-2")
	err = s.State.AddCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2.String())

	// Use a URL with an arbitrary rev to search.
	curl := charm.MustParseURL("ch:quantal/dummy-23")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pending.URL(), gc.Equals, curl2.String())
	c.Assert(pending.IsPlaceholder(), jc.IsTrue)
	c.Assert(pending.Meta(), gc.IsNil)
	c.Assert(pending.Config(), gc.IsNil)
	c.Assert(pending.StoragePath(), gc.Equals, "")
	c.Assert(pending.BundleSha256(), gc.Equals, "")
}

func (s *CharmSuite) TestAddCharmPlaceholderErrors(c *gc.C) {
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	err := s.State.AddCharmPlaceholder(curl)
	c.Assert(err, gc.ErrorMatches, "expected charm URL with a valid schema, got .*")

	curl = charm.MustParseURL("ch:quantal/dummy")
	err = s.State.AddCharmPlaceholder(curl)
	c.Assert(err, gc.ErrorMatches, "expected charm URL with revision, got .*")
}

func (s *CharmSuite) TestAddCharmPlaceholder(c *gc.C) {
	curl := charm.MustParseURL("ch:quantal/dummy-1")
	err := s.State.AddCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl.String())

	// Add the same one again, should be a no-op
	err = s.State.AddCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl.String())
}

func (s *CharmSuite) assertAddCharmPlaceholder(c *gc.C) (string, *charm.URL, *state.Charm) {
	// Add a deployed charm
	info := s.dummyCharm(c, "ch:quantal/dummy-1")
	dummy, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Add a charm placeholder
	curl2 := charm.MustParseURL("ch:quantal/dummy-2")
	err = s.State.AddCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl2.String())

	// Deployed charm is still there.
	existing, err := s.State.Charm(info.ID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(existing, jc.DeepEquals, dummy)

	return info.ID, curl2, dummy
}

func (s *CharmSuite) TestAddCharmPlaceholderLeavesDeployedCharmsAlone(c *gc.C) {
	s.assertAddCharmPlaceholder(c)
}

func (s *CharmSuite) TestAddCharmPlaceholderDeletesOlder(c *gc.C) {
	curl, curlOldRef, dummy := s.assertAddCharmPlaceholder(c)

	// Add a new charm placeholder
	curl3 := charm.MustParseURL("ch:quantal/dummy-3")
	err := s.State.AddCharmPlaceholder(curl3)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPlaceholderCharmExists(c, curl3.String())

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
	info := s.dummyCharm(c, "ch:quantal/dummy-1")
	sch, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// Add a charm reference
	curl2 := charm.MustParseURL("ch:quantal/dummy-2")
	err = s.State.AddCharmPlaceholder(curl2)
	c.Assert(err, jc.ErrorIsNil)

	charms, err := s.State.AllCharms()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charms, gc.HasLen, 3)

	c.Assert(charms[0].URL(), gc.Equals, "local:quantal/quantal-dummy-1")
	c.Assert(charms[1], gc.DeepEquals, sch)
	c.Assert(charms[2].URL(), gc.Equals, curl2.String())
}

func (s *CharmSuite) TestAddCharmMetadata(c *gc.C) {
	// Check that a charm with missing sha/storage path is flagged as pending
	// to be uploaded.
	dummy1 := s.dummyCharm(c, "ch:quantal/dummy-1")
	dummy1.SHA256 = ""
	dummy1.StoragePath = ""
	ch1, err := s.State.AddCharmMetadata(dummy1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch1.IsPlaceholder(), jc.IsFalse)
	c.Check(ch1.IsUploaded(), jc.IsFalse, gc.Commentf("expected charm with missing SHA/storage path to have the PendingUpload flag set"))

	// Check that uploading the same charm ID yields the same charm
	ch, err := s.State.AddCharmMetadata(dummy1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch1, gc.DeepEquals, ch)

	// Check that a charm with populated sha/storage path is flagged as
	// uploaded.
	dummy2 := s.dummyCharm(c, "ch:quantal/dummy-2")
	ch2, err := s.State.AddCharmMetadata(dummy2)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch2.IsPlaceholder(), jc.IsFalse)
	c.Check(ch2.IsUploaded(), jc.IsTrue, gc.Commentf("expected charm with populated SHA/storage path to have the PendingUpload flag unset"))
}

func (s *CharmSuite) TestAddCharmMetadataUpdatesPlaceholder(c *gc.C) {
	// The charm revision updater adds a placeholder charm doc into the db.
	// Ensure that AddCharmMetadata can handle that.
	err := s.State.AddCharmPlaceholder(charm.MustParseURL("ch:quantal/testme-2"))
	c.Assert(err, jc.ErrorIsNil)

	testme := s.dummyCharm(c, "ch:quantal/testme-2")
	ch2, err := s.State.AddCharmMetadata(testme)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch2.IsPlaceholder(), jc.IsFalse)
}

func (s *CharmSuite) TestAllCharmURLs(c *gc.C) {
	ch2 := state.AddTestingCharmhubCharmForSeries(c, s.State, "jammy", "dummy")
	state.AddTestingApplication(c, s.State, "testme-jammy", ch2)

	curls, err := s.State.AllCharmURLs()
	c.Assert(err, jc.ErrorIsNil)
	// One application from SetUpTest
	c.Assert(len(curls), gc.Equals, 2, gc.Commentf("%v", curls))
}

type CharmTestHelperSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CharmTestHelperSuite{})

func assertCustomCharm(
	c *gc.C,
	ch *state.Charm,
	series string,
	meta *charm.Meta,
	config *charm.Config,
	metrics *charm.Metrics,
	revision int,
) {
	// Check Charm interface method results.
	c.Assert(ch.Meta(), gc.DeepEquals, meta)
	c.Assert(ch.Config(), gc.DeepEquals, config)
	c.Assert(ch.Metrics(), gc.DeepEquals, metrics)
	c.Assert(ch.Revision(), gc.DeepEquals, revision)

	// Test URL matches charm and expected series.
	url := charm.MustParseURL(ch.URL())
	c.Assert(url.Series, gc.Equals, series)
	c.Assert(url.Revision, gc.Equals, ch.Revision())

	// Ignore the StoragePath and BundleSHA256 methods, they're irrelevant.
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

		ch = s.AddSeriesCharm(c, name, "bionic")
		assertCustomCharm(c, ch, "bionic", meta, config, metrics, revision)
	})
}

var configYaml = `
options:
  working:
    description: when set to false, prevents application from functioning correctly
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
	forEachStandardCharm(c, func(name string) {
		actions, err := charm.ReadActionsYaml(name, bytes.NewBuffer([]byte(actionsYaml)))
		c.Assert(err, jc.ErrorIsNil)
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

func (s *CharmTestHelperSuite) TestLXDProfileCharm(c *gc.C) {
	chd := testcharms.Repo.CharmDir("lxd-profile")
	c.Assert(chd.LXDProfile(), jc.DeepEquals, &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting":       "true",
			"security.privileged":    "true",
			"linux.kernel_modules":   "openvswitch,nbd,ip_tables,ip6_tables",
			"environment.http_proxy": "",
		},
		Description: "lxd profile for testing, will pass validation",
		Devices: map[string]map[string]string{
			"tun": {
				"path": "/dev/net/tun",
				"type": "unix-char",
			},
			"sony": {
				"type":      "usb",
				"vendorid":  "0fce",
				"productid": "51da",
			},
			"bdisk": {
				"source": "/dev/loop0",
				"type":   "unix-block",
			},
			"gpu": {
				"type": "gpu",
			},
		},
	})
}

var manifestYaml = `
bases:
  - name: ubuntu
    channel: "18.04"
  - name: ubuntu
    channel: "20.04"
`

func (s *CharmTestHelperSuite) TestManifestCharm(c *gc.C) {
	manifest, err := charm.ReadManifest(bytes.NewBuffer([]byte(manifestYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		ch := s.AddManifestCharm(c, name, manifestYaml, 123)
		c.Assert(ch.Manifest(), gc.DeepEquals, manifest)
	})
}

func (s *CharmTestHelperSuite) TestTestingCharm(c *gc.C) {
	added := state.AddTestingCharmFromRepo(c, s.State, "metered", testcharms.CharmRepo())
	c.Assert(added.Metrics(), gc.NotNil)

	charmDir := testcharms.CharmRepo().CharmDir("metered")
	c.Assert(charmDir.Metrics(), gc.DeepEquals, added.Metrics())
}
