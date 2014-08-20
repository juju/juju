// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"

	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v3"
	charmtesting "gopkg.in/juju/charm.v3/testing"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/instance"
)

var (
	GetBackupMetadata   = getBackupMetadata
	AddBackupMetadata   = addBackupMetadata
	AddBackupMetadataID = addBackupMetadataID
	SetBackupStored     = setBackupStored
)

func SetTestHooks(c *gc.C, st *State, hooks ...jujutxn.TestHook) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetTestHooks(c, runner, hooks...)
}

func SetBeforeHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetBeforeHooks(c, runner, fs...)
}

func SetAfterHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetAfterHooks(c, runner, fs...)
}

func SetRetryHooks(c *gc.C, st *State, block, check func()) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetRetryHooks(c, runner, block, check)
}

// SetPolicy updates the State's policy field to the
// given Policy, and returns the old value.
func SetPolicy(st *State, p Policy) Policy {
	old := st.policy
	st.policy = p
	return old
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
	settingsRefsCollection, closer := st.getCollection(settingsrefsC)
	defer closer()

	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := settingsRefsCollection.FindId(key).One(&doc); err == nil {
		return doc.RefCount, nil
	}
	return 0, mgo.ErrNotFound
}

func AddTestingCharm(c *gc.C, st *State, name string) *Charm {
	return addCharm(c, st, "quantal", charmtesting.Charms.CharmDir(name))
}

func AddTestingService(c *gc.C, st *State, name string, ch *Charm) *Service {
	c.Assert(ch, gc.NotNil)
	return AddTestingServiceWithNetworks(c, st, name, ch, nil)
}

func AddTestingServiceWithNetworks(c *gc.C, st *State, name string, ch *Charm, networks []string) *Service {
	c.Assert(ch, gc.NotNil)
	service, err := st.AddService(name, "user-admin", ch, networks)
	c.Assert(err, gc.IsNil)
	return service
}

func AddCustomCharm(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	path := charmtesting.Charms.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		config := filepath.Join(path, filename)
		err := ioutil.WriteFile(config, []byte(content), 0644)
		c.Assert(err, gc.IsNil)
	}
	ch, err := charm.ReadCharmDir(path)
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
			C:      instanceDataC,
			Id:     m.doc.Id,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"instanceid", ""}}}},
		},
	}

	err := m.st.runTransaction(ops)
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

// TxnRevno returns the txn-revno field of the document
// associated with the given Id in the given collection.
func TxnRevno(st *State, coll string, id interface{}) (int64, error) {
	var doc struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	err := st.db.C(coll).FindId(id).One(&doc)
	if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

// MinUnitsRevno returns the Revno of the minUnits document
// associated with the given service name.
func MinUnitsRevno(st *State, serviceName string) (int, error) {
	minUnitsCollection, closer := st.getCollection(minUnitsC)
	defer closer()
	var doc minUnitsDoc
	if err := minUnitsCollection.FindId(serviceName).One(&doc); err != nil {
		return 0, err
	}
	return doc.Revno, nil
}

func ParseTag(st *State, tag names.Tag) (string, string, error) {
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

func EnsureActionMarker(prefix string) string {
	return ensureActionMarker(prefix)
}

var EnsureActionResultMarker = ensureSuffixFn(actionResultMarker)

func GetActionResultId(actionId string) (string, bool) {
	return convertActionIdToActionResultId(actionId)
}

func WatcherMergeIds(changes, initial set.Strings, updates map[interface{}]bool) error {
	return mergeIds(changes, initial, updates)
}

func WatcherEnsureSuffixFn(marker string) func(string) string {
	return ensureSuffixFn(marker)
}

func WatcherMakeIdFilter(marker string, receivers ...ActionReceiver) func(interface{}) bool {
	return makeIdFilter(marker, receivers...)
}

var (
	GetOrCreatePorts = getOrCreatePorts
	GetPorts         = getPorts
	NowToTheSecond   = nowToTheSecond
)
