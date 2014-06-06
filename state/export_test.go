// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/charm"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
)

// SetPolicy updates the State's policy field to the
// given Policy, and returns the old value.
func SetPolicy(st *State, p Policy) Policy {
	old := st.policy
	st.policy = p
	return old
}

// TestingInitialize initializes the state and returns it. If state was not
// already initialized, and cfg is nil, the minimal default environment
// configuration will be used.
func TestingInitialize(c *gc.C, cfg *config.Config, policy Policy) *State {
	if cfg == nil {
		cfg = testing.EnvironConfig(c)
	}
	st, err := Initialize(TestingStateInfo(), cfg, TestingDialOpts(), policy)
	c.Assert(err, gc.IsNil)
	return st
}

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

func AddTestingCharm(c *gc.C, st *State, name string) *Charm {
	return addCharm(c, st, "quantal", testing.Charms.Dir(name))
}

func AddTestingService(c *gc.C, st *State, name string, ch *Charm) *Service {
	return AddTestingServiceWithNetworks(c, st, name, ch, nil, nil)
}

func AddTestingServiceWithNetworks(c *gc.C, st *State, name string, ch *Charm, includeNetworks, excludeNetworks []string) *Service {
	service, err := st.AddService(name, "user-admin", ch, includeNetworks, excludeNetworks)
	c.Assert(err, gc.IsNil)
	return service
}

func AddCustomCharm(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	path := testing.Charms.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		config := filepath.Join(path, filename)
		err := ioutil.WriteFile(config, []byte(content), 0644)
		c.Assert(err, gc.IsNil)
	}
	ch, err := charm.ReadDir(path)
	c.Assert(err, gc.IsNil)
	if revision != -1 {
		ch.SetRevision(revision)
	}
	return addCharm(c, st, series, ch)
}

func addCharm(c *gc.C, st *State, series string, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%s-%d", series, ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:" + series + "/" + ident)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/" + ident)
	c.Assert(err, gc.IsNil)
	sch, err := st.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, gc.IsNil)
	return sch
}

var MachineIdLessThan = machineIdLessThan

var JobNames = jobNames

// SCHEMACHANGE
// This method is used to reset a deprecated machine attribute.
func SetMachineInstanceId(m *Machine, instanceId string) {
	m.doc.InstanceId = instance.Id(instanceId)
}

// SCHEMACHANGE
// ClearInstanceDocId sets instanceid on instanceData for machine to "".
func ClearInstanceDocId(c *gc.C, m *Machine) {
	ops := []txn.Op{
		{
			C:      m.st.instanceData.Name,
			Id:     m.doc.Id,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"instanceid", ""}}}},
		},
	}

	err := m.st.RunTransaction(ops)
	c.Assert(err, gc.IsNil)
}

// SCHEMACHANGE
// This method is used to reset the ownertag attribute
func SetServiceOwnerTag(s *Service, ownerTag string) {
	s.doc.OwnerTag = ownerTag
}

// SCHEMACHANGE
// Get the owner directly
func GetServiceOwnerTag(s *Service) string {
	return s.doc.OwnerTag
}

func SetPasswordHash(e Authenticator, passwordHash string) error {
	type hasSetPasswordHash interface {
		setPasswordHash(string) error
	}
	return e.(hasSetPasswordHash).setPasswordHash(passwordHash)
}

// Return the underlying PasswordHash stored in the database. Used by the test
// suite to check that the PasswordHash gets properly updated to new values
// when compatibility mode is detected.
func GetPasswordHash(e Authenticator) string {
	type hasGetPasswordHash interface {
		getPasswordHash() string
	}
	return e.(hasGetPasswordHash).getPasswordHash()
}

func init() {
	logSize = logSizeTests
}

// MinUnitsRevno returns the Revno of the minUnits document
// associated with the given service name.
func MinUnitsRevno(st *State, serviceName string) (int, error) {
	var doc minUnitsDoc
	if err := st.minUnits.FindId(serviceName).One(&doc); err != nil {
		return 0, err
	}
	return doc.Revno, nil
}

func ParseTag(st *State, tag string) (string, string, error) {
	return st.parseTag(tag)
}

// Return the PasswordSalt that goes along with the PasswordHash
func GetUserPasswordSaltAndHash(u *User) (string, string) {
	return u.doc.PasswordSalt, u.doc.PasswordHash
}

var NewAddress = newAddress

func CheckUserExists(st *State, name string) (bool, error) {
	return st.checkUserExists(name)
}

var StateServerAvailable = &stateServerAvailable

func UnitConstraints(u *Unit) (*constraints.Value, error) {
	return u.constraints()
}
