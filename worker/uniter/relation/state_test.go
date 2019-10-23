// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/relation"
)

type StateDirSuite struct{}

type msi map[string]int64

var _ = gc.Suite(&StateDirSuite{})

func (s *StateDirSuite) TestReadStateDirEmpty(c *gc.C) {
	basedir := c.MkDir()
	reldir := filepath.Join(basedir, "123")

	dir, err := relation.ReadStateDir(basedir, 123)
	c.Assert(err, jc.ErrorIsNil)
	state := dir.State()
	c.Assert(state.RelationId, gc.Equals, 123)
	c.Assert(msi(state.Members), gc.DeepEquals, msi{})
	c.Assert(msi(state.ApplicationMembers), gc.DeepEquals, msi{})
	c.Assert(state.ChangedPending, gc.Equals, "")

	_, err = os.Stat(reldir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	exists := dir.Exists()
	c.Assert(exists, jc.IsFalse)
	err = dir.Ensure()
	c.Assert(err, jc.ErrorIsNil)
	exists = dir.Exists()
	c.Assert(exists, jc.IsTrue)
	fi, err := os.Stat(reldir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fi, jc.Satisfies, os.FileInfo.IsDir)
}

func (s *StateDirSuite) TestReadStateDirValid(c *gc.C) {
	basedir := c.MkDir()
	reldir := setUpDir(c, basedir, "123", map[string]string{
		"foo-bar-1":           "change-version: 99\n",
		"foo-bar-1.preparing": "change-version: 100\n",
		"baz-qux-7":           "change-version: 101\nchanged-pending: true\n",
		"nonsensical":         "blah",
		"27":                  "blah",
	})
	setUpDir(c, reldir, "ignored", nil)

	dir, err := relation.ReadStateDir(basedir, 123)
	c.Assert(err, jc.ErrorIsNil)
	state := dir.State()
	c.Assert(state.RelationId, gc.Equals, 123)
	c.Assert(msi(state.Members), gc.DeepEquals, msi{"foo-bar/1": 99, "baz-qux/7": 101})
	c.Assert(state.ChangedPending, gc.Equals, "baz-qux/7")
}

var badRelationsTests = []struct {
	contents map[string]string
	subdirs  []string
	err      string
}{
	{
		nil, []string{"foo-bar-1"},
		`.* (is a directory|handle is invalid.)`,
	}, {
		map[string]string{"foo-1": "'"}, nil,
		`invalid unit file "foo-1": yaml: found unexpected end of stream`,
	}, {
		map[string]string{"foo-1": "blah: blah\n"}, nil,
		`invalid unit file "foo-1": "changed-version" not set`,
	}, {
		map[string]string{
			"foo-1": "change-version: 123\nchanged-pending: true\n",
			"foo-2": "change-version: 456\nchanged-pending: true\n",
		}, nil,
		`"foo/1" and "foo/2" both have pending changed hooks`,
	},
}

func (s *StateDirSuite) TestBadRelations(c *gc.C) {
	for i, t := range badRelationsTests {
		c.Logf("test %d", i)
		basedir := c.MkDir()
		reldir := setUpDir(c, basedir, "123", t.contents)
		for _, subdir := range t.subdirs {
			setUpDir(c, reldir, subdir, nil)
		}
		_, err := relation.ReadStateDir(basedir, 123)
		expect := `cannot load relation state from ".*": ` + t.err
		c.Assert(err, gc.ErrorMatches, expect)
	}
}

var defaultMembers = msi{"foo/1": 0, "foo/2": 0}
var defaultAppMembers = msi{"foo": 0}

// writeTests verify the behaviour of sequences of HookInfos on a relation
// state that starts off containing defaultMembers.
var writeTests = []struct {
	description string
	hooks       []hook.Info
	members     msi
	appMembers  msi
	pending     string
	err         string
	deleted     bool
}{
	// Verify that valid changes work.
	{
		description: "relation-changed foo/1 to version 1",
		hooks: []hook.Info{{
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
			ChangeVersion:     1,
		}},
		members: msi{"foo/1": 1, "foo/2": 0},
	}, {
		description: "relation-joined foo/3 at 0",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
	}, {
		description: "relation-joined foo/3 and relation-changed foo/3",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
	}, {
		description: "relation-departed foo/1",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/2": 0},
	}, {
		description: "relation-departed foo/1 and relation-joined foo/1",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0},
		pending: "foo/1",
	}, {
		description: "relation-departed foo/1 and relation-joined foo/1 and relation-changed foo/1",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}, {
			Kind:       hooks.RelationJoined,
			RelationId: 123,
			RemoteUnit: "foo/1",
		}, {
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0},
	}, {
		description: "relation-departed foo/1 and relation-departed foo/2 and relation-broken",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/2",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationBroken,
			RelationId:        123,
			RemoteApplication: "foo",
		}},
		deleted: true,
	},
	// Verify detection of various error conditions.
	{
		description: "wrong relation id",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        456,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		err: "expected relation 123, got relation 456",
	}, {
		description: "relation-changed foo/3 must follow relation-joined foo/3 (before relation-joined of another unit)",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/4",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
		err:     `expected "relation-changed" for "foo/3"`,
	}, {
		description: "relation-changed foo/3 must follow relation-joined foo/3 (not relation-changed for another unit)",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
		err:     `expected "relation-changed" for "foo/3"`,
	}, {
		description: "relation-joined of a joined unit",
		hooks: []hook.Info{{
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		err: "unit already joined",
	}, {
		description: "relation-changed of an unjoined unit",
		hooks: []hook.Info{{
			Kind:              hooks.RelationChanged,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		err: "unit has not joined",
	}, {
		description: "relation-departed of a non-existent unit",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/3",
			RemoteApplication: "foo",
		}},
		err: "unit has not joined",
	}, {
		description: "relation-broken with existing units",
		hooks: []hook.Info{{
			Kind:       hooks.RelationBroken,
			RelationId: 123,
		}},
		err: `cannot run "relation-broken" while units still present`,
	}, {
		description: "relation-joined after relation has been broken",
		hooks: []hook.Info{{
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationDeparted,
			RelationId:        123,
			RemoteUnit:        "foo/2",
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationBroken,
			RelationId:        123,
			RemoteApplication: "foo",
		}, {
			Kind:              hooks.RelationJoined,
			RelationId:        123,
			RemoteUnit:        "foo/1",
			RemoteApplication: "foo",
		}},
		err:     `relation is broken and cannot be changed further`,
		deleted: true,
	},
}

func (s *StateDirSuite) TestWrite(c *gc.C) {
	for i, t := range writeTests {
		c.Logf("test %d: %v", i, t.description)
		basedir := c.MkDir()
		setUpDir(c, basedir, "123", map[string]string{
			"foo-1":   "change-version: 0\n",
			"foo-2":   "change-version: 0\n",
			"foo-app": "change-version: 0\n",
		})
		dir, err := relation.ReadStateDir(basedir, 123)
		c.Assert(err, jc.ErrorIsNil)
		for i, hi := range t.hooks {
			c.Logf("  hook %d %v %v %v", i, hi.Kind, hi.RemoteUnit, hi.RemoteApplication)
			if i == len(t.hooks)-1 && t.err != "" {
				err = dir.State().Validate(hi)
				expect := fmt.Sprintf(`inappropriate %q for %q: %s`, hi.Kind, hi.RemoteUnit, t.err)
				c.Assert(err, gc.ErrorMatches, expect)
			} else {
				err = dir.State().Validate(hi)
				c.Assert(err, jc.ErrorIsNil)
				err = dir.Write(hi)
				c.Assert(err, jc.ErrorIsNil)
				// Check that writing the same change again is OK.
				err = dir.Write(hi)
				c.Assert(err, jc.ErrorIsNil)
			}
		}
		members := t.members
		if members == nil && !t.deleted {
			members = defaultMembers
		}
		appMembers := t.appMembers
		if appMembers == nil && !t.deleted {
			appMembers = defaultAppMembers
		}
		assertState(c, dir, basedir, 123, members, appMembers, t.pending, t.deleted)
	}
}

func (s *StateDirSuite) TestRemove(c *gc.C) {
	basedir := c.MkDir()
	dir, err := relation.ReadStateDir(basedir, 1)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.Ensure()
	c.Assert(err, jc.ErrorIsNil)
	err = dir.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = dir.Remove()
	c.Assert(err, jc.ErrorIsNil)

	setUpDir(c, basedir, "99", map[string]string{
		"foo-1": "change-version: 0\n",
	})
	dir, err = relation.ReadStateDir(basedir, 99)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.Remove()
	// Windows message is The directory is not empty
	// Unix message is directory not empty
	c.Assert(err, gc.ErrorMatches, ".* directory (is )?not empty.?")
}

type ReadAllStateDirsSuite struct{}

var _ = gc.Suite(&ReadAllStateDirsSuite{})

func (s *ReadAllStateDirsSuite) TestNoDir(c *gc.C) {
	basedir := c.MkDir()
	relsdir := filepath.Join(basedir, "relations")

	dirs, err := relation.ReadAllStateDirs(relsdir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirs, gc.HasLen, 0)

	_, err = os.Stat(relsdir)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *ReadAllStateDirsSuite) TestBadStateDir(c *gc.C) {
	basedir := c.MkDir()
	relsdir := setUpDir(c, basedir, "relations", nil)
	setUpDir(c, relsdir, "123", map[string]string{
		"bad-0": "blah: blah\n",
	})
	_, err := relation.ReadAllStateDirs(relsdir)
	c.Assert(err, gc.ErrorMatches, `cannot load relations state from .*: cannot load relation state from .*: invalid unit file "bad-0": "changed-version" not set`)
}

func (s *ReadAllStateDirsSuite) TestReadAllStateDirs(c *gc.C) {
	basedir := c.MkDir()
	relsdir := setUpDir(c, basedir, "relations", map[string]string{
		"ignored":     "blah",
		"foo-bar-123": "gibberish",
	})
	setUpDir(c, relsdir, "123", map[string]string{
		"foo-0":     "change-version: 1\n",
		"foo-1":     "change-version: 2\nchanged-pending: true\n",
		"gibberish": "gibberish",
	})
	setUpDir(c, relsdir, "456", map[string]string{
		"bar-0": "change-version: 3\n",
		"bar-1": "change-version: 4\n",
	})
	setUpDir(c, relsdir, "789", nil)
	setUpDir(c, relsdir, "10", map[string]string{
		"baz-app": "change-version: 2\n",
	})
	setUpDir(c, relsdir, "onethousand", map[string]string{
		"baz-0": "change-version: 3\n",
		"baz-1": "change-version: 4\n",
	})

	dirs, err := relation.ReadAllStateDirs(relsdir)
	c.Assert(err, jc.ErrorIsNil)
	for id, dir := range dirs {
		c.Logf("%d: %#v", id, dir)
	}
	assertState(c, dirs[123], relsdir, 123, msi{"foo/0": 1, "foo/1": 2}, msi{}, "foo/1", false)
	assertState(c, dirs[456], relsdir, 456, msi{"bar/0": 3, "bar/1": 4}, msi{}, "", false)
	assertState(c, dirs[789], relsdir, 789, msi{}, msi{}, "", false)
	assertState(c, dirs[10], relsdir, 10, msi{}, msi{"baz": 2}, "", false)
	c.Assert(dirs, gc.HasLen, 4)
}

func setUpDir(c *gc.C, basedir, name string, contents map[string]string) string {
	reldir := filepath.Join(basedir, name)
	err := os.Mkdir(reldir, 0777)
	c.Assert(err, jc.ErrorIsNil)
	for name, content := range contents {
		path := filepath.Join(reldir, name)
		err := ioutil.WriteFile(path, []byte(content), 0777)
		c.Assert(err, jc.ErrorIsNil)
	}
	return reldir
}

func assertState(c *gc.C, dir *relation.StateDir, relsdir string, relationId int, members msi, appmembers msi, pending string, deleted bool) {
	expect := &relation.State{
		RelationId:         relationId,
		Members:            map[string]int64(members),
		ApplicationMembers: map[string]int64(appmembers),
		ChangedPending:     pending,
	}
	c.Assert(dir.State(), gc.DeepEquals, expect)
	if deleted {
		_, err := os.Stat(filepath.Join(relsdir, strconv.Itoa(relationId)))
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	} else {
		fresh, err := relation.ReadStateDir(relsdir, relationId)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(fresh.State(), gc.DeepEquals, expect)
	}
}
