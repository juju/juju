package uniter_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/worker/uniter"
	"os"
	"path/filepath"
)

type RelationStateSuite struct{}

var _ = Suite(&RelationStateSuite{})

func (s *RelationStateSuite) TestNewRelationStateEmpty(c *C) {
	basedir := c.MkDir()
	reldir := filepath.Join(basedir, "123")

	rs, err := uniter.NewRelationState(basedir, 123)
	c.Assert(err, IsNil)
	c.Assert(rs.Path, Equals, reldir)
	c.Assert(rs.RelationId, Equals, 123)
	c.Assert(msi(rs.Members), DeepEquals, msi{})
	c.Assert(rs.ChangedPending, Equals, "")

	fi, err := os.Stat(reldir)
	c.Assert(err, IsNil)
	c.Assert(fi.IsDir(), Equals, true)
}

func (s *RelationStateSuite) TestNewRelationStateValid(c *C) {
	basedir := c.MkDir()
	reldir := setUpDir(c, basedir, "123", map[string]string{
		"foo-bar-1":           "change-version: 99\n",
		"foo-bar-1.preparing": "change-version: 100\n",
		"baz-qux-7":           "change-version: 101\nchanged-pending: true\n",
		"nonsensical":         "blah",
		"27":                  "blah",
	})
	setUpDir(c, reldir, "ignored", nil)

	rs, err := uniter.NewRelationState(basedir, 123)
	c.Assert(err, IsNil)
	c.Assert(rs.Path, Equals, reldir)
	c.Assert(rs.RelationId, Equals, 123)
	c.Assert(msi(rs.Members), DeepEquals, msi{"foo-bar/1": 99, "baz-qux/7": 101})
	c.Assert(rs.ChangedPending, Equals, "baz-qux/7")
}

var badRelationsTests = []struct {
	contents map[string]string
	subdirs  []string
	err      string
}{
	{
		nil, []string{"foo-bar-1"},
		`.* is a directory`,
	}, {
		map[string]string{"foo-1": "'"}, nil,
		`invalid unit file "foo-1": YAML error: .*`,
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

func (s *RelationStateSuite) TestBadRelations(c *C) {
	for i, t := range badRelationsTests {
		c.Logf("test %d", i)
		basedir := c.MkDir()
		reldir := setUpDir(c, basedir, "123", t.contents)
		for _, subdir := range t.subdirs {
			setUpDir(c, reldir, subdir, nil)
		}
		_, err := uniter.NewRelationState(basedir, 123)
		expect := `cannot load relation state from ".*": ` + t.err
		c.Assert(err, ErrorMatches, expect)
	}
}

var defaultMembers = msi{"foo/1": 0, "foo/2": 0}

// commitTests verify the behaviour of sequences of HookInfos on a relation
// state that starts off containing defaultMembers.
var commitTests = []struct {
	hooks   []uniter.HookInfo
	members msi
	pending string
	err     string
	deleted bool
}{
	// Verify that valid changes work.
	{
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "changed", RemoteUnit: "foo/1", ChangeVersion: 1},
		},
		members: msi{"foo/1": 1, "foo/2": 0},
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/3"},
		},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/3"},
			{RelationId: 123, HookKind: "changed", RemoteUnit: "foo/3"},
		},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/1"},
		},
		members: msi{"foo/2": 0},
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/1"},
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/1"},
		},
		members: msi{"foo/1": 0, "foo/2": 0},
		pending: "foo/1",
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/1"},
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/1"},
			{RelationId: 123, HookKind: "changed", RemoteUnit: "foo/1"},
		},
		members: msi{"foo/1": 0, "foo/2": 0},
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/1"},
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/2"},
			{RelationId: 123, HookKind: "broken"},
		},
		deleted: true,
	},
	// Verify detection of various error conditions.
	{
		hooks: []uniter.HookInfo{
			{RelationId: 456, HookKind: "joined", RemoteUnit: "foo/1"},
		},
		err: "expected relation 123, got relation 456",
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/3"},
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/4"},
		},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
		err:     `expected "changed" for "foo/3"`,
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/3"},
			{RelationId: 123, HookKind: "changed", RemoteUnit: "foo/1"},
		},
		members: msi{"foo/1": 0, "foo/2": 0, "foo/3": 0},
		pending: "foo/3",
		err:     `expected "changed" for "foo/3"`,
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/1"},
		},
		err: "unit already joined",
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "changed", RemoteUnit: "foo/3"},
		},
		err: "unit has not joined",
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/3"},
		},
		err: "unit has not joined",
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "broken"},
		},
		err: `cannot run "broken" while units still present`,
	}, {
		hooks: []uniter.HookInfo{
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/1"},
			{RelationId: 123, HookKind: "departed", RemoteUnit: "foo/2"},
			{RelationId: 123, HookKind: "broken"},
			{RelationId: 123, HookKind: "joined", RemoteUnit: "foo/1"},
		},
		err:     `relation is broken and cannot be changed further`,
		deleted: true,
	},
}

func (s *RelationStateSuite) TestCommit(c *C) {
	for i, t := range commitTests {
		c.Logf("test %d", i)
		basedir := c.MkDir()
		setUpDir(c, basedir, "123", map[string]string{
			"foo-1": "change-version: 0\n",
			"foo-2": "change-version: 0\n",
		})
		rs, err := uniter.NewRelationState(basedir, 123)
		c.Assert(err, IsNil)
		for i, hi := range t.hooks {
			c.Logf("  hook %d", i)
			if i == len(t.hooks)-1 && t.err != "" {
				err = rs.Validate(hi)
				expect := fmt.Sprintf(`inappropriate %q for %q: %s`, hi.HookKind, hi.RemoteUnit, t.err)
				c.Assert(err, ErrorMatches, expect)
			} else {
				err = rs.Validate(hi)
				c.Assert(err, IsNil)
				err = rs.Commit(hi)
				c.Assert(err, IsNil)
				// Check that committing the same change again is OK.
				err = rs.Commit(hi)
				c.Assert(err, IsNil)
			}
		}
		members := t.members
		if members == nil && !t.deleted {
			members = defaultMembers
		}
		assertState(c, rs, members, t.pending, t.deleted)
	}
}

type AllRelationStatesSuite struct{}

var _ = Suite(&AllRelationStatesSuite{})

func (s *AllRelationStatesSuite) TestNoExist(c *C) {
	basedir := c.MkDir()
	relsdir := filepath.Join(basedir, "relations")

	states, err := uniter.AllRelationStates(relsdir)
	c.Assert(err, IsNil)
	c.Assert(states, HasLen, 0)

	fi, err := os.Stat(relsdir)
	c.Assert(err, IsNil)
	c.Assert(fi.IsDir(), Equals, true)
}

func (s *AllRelationStatesSuite) TestBadRelationState(c *C) {
	basedir := c.MkDir()
	relsdir := setUpDir(c, basedir, "relations", nil)
	setUpDir(c, relsdir, "123", map[string]string{
		"bad-0": "blah: blah\n",
	})
	_, err := uniter.AllRelationStates(relsdir)
	c.Assert(err, ErrorMatches, `cannot load relations state from .*: cannot load relation state from .*: invalid unit file "bad-0": "changed-version" not set`)
}

func (s *AllRelationStatesSuite) TestAllRelationStates(c *C) {
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
	setUpDir(c, relsdir, "onethousand", map[string]string{
		"baz-0": "change-version: 3\n",
		"baz-1": "change-version: 4\n",
	})

	states, err := uniter.AllRelationStates(relsdir)
	c.Assert(err, IsNil)
	for id, rs := range states {
		c.Logf("%d: %#v", id, rs)
	}
	assertState(c, states[123], msi{"foo/0": 1, "foo/1": 2}, "foo/1", false)
	assertState(c, states[456], msi{"bar/0": 3, "bar/1": 4}, "", false)
	assertState(c, states[789], msi{}, "", false)
	c.Assert(states, HasLen, 3)
}

func setUpDir(c *C, basedir, name string, contents map[string]string) string {
	reldir := filepath.Join(basedir, name)
	err := os.Mkdir(reldir, 0777)
	c.Assert(err, IsNil)
	for name, content := range contents {
		path := filepath.Join(reldir, name)
		err := ioutil.WriteFile(path, []byte(content), 0777)
		c.Assert(err, IsNil)
	}
	return reldir
}

func assertState(c *C, rs *uniter.RelationState, members msi, pending string, deleted bool) {
	expect := &uniter.RelationState{
		Path:           rs.Path,
		RelationId:     rs.RelationId,
		Members:        map[string]int(members),
		ChangedPending: pending,
	}
	c.Assert(rs, DeepEquals, expect)
	if deleted {
		_, err := os.Stat(rs.Path)
		c.Assert(os.IsNotExist(err), Equals, true)
	} else {
		basedir := filepath.Dir(rs.Path)
		committed, err := uniter.NewRelationState(basedir, rs.RelationId)
		c.Assert(err, IsNil)
		c.Assert(committed, DeepEquals, expect)
	}
}
