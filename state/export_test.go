package state

import (
	"fmt"
	"io/ioutil"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"net/url"
	"path/filepath"
	"time"
)

// TestingEnvironConfig returns a default environment configuration.
func TestingEnvironConfig(c *C) *config.Config {
	cfg, err := config.New(map[string]interface{}{
		"type":            "test",
		"name":            "test-name",
		"default-series":  "test-series",
		"authorized-keys": "test-keys",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, IsNil)
	return cfg
}

// TestingInitialize ensures that state has been initialized. If state was not
// already initialized, and cfg is nil, the minimal default environment
// configuration will be used.
func TestingInitialize(c *C, cfg *config.Config) {
	if cfg == nil {
		cfg = TestingEnvironConfig(c)
	}
	st, err := Initialize(TestingStateInfo(), cfg, TestingDialTimeout)
	c.Assert(err, IsNil)
	err = st.Close()
	c.Assert(err, IsNil)
}

// TestingDialTimeout controls how long calls to state.Open
// will wait during testing.
const TestingDialTimeout = 100 * time.Millisecond

type (
	CharmDoc    charmDoc
	MachineDoc  machineDoc
	RelationDoc relationDoc
	ServiceDoc  serviceDoc
	UnitDoc     unitDoc
)

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

func ServiceSettingsRefCount(st *State, serviceName string, curl *charm.URL) (int, error) {
	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := st.settingsrefs.FindId(key).One(&doc); err == nil {
		return doc.RefCount, nil
	}
	return 0, mgo.ErrNotFound
}

func AddTestingCharm(c *C, st *State, name string) *Charm {
	return addCharm(c, st, "series", testing.Charms.Dir(name))
}

func AddCustomCharm(c *C, st *State, name, filename, content, series string, revision int) *Charm {
	path := testing.Charms.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		config := filepath.Join(path, filename)
		err := ioutil.WriteFile(config, []byte(content), 0644)
		c.Assert(err, IsNil)
	}
	ch, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	if revision != -1 {
		ch.SetRevision(revision)
	}
	return addCharm(c, st, series, ch)
}

func addCharm(c *C, st *State, series string, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%s-%d", series, ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:" + series + "/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := st.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}

func init() {
	logSize = logSizeTests
}
