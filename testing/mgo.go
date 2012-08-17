package testing

import (
	"fmt"
	"io/ioutil"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"os"
	"os/exec"
	"strconv"
	stdtesting "testing"
)

// MgoAddr holds the address of the shared MongoDB server set up by
// MgoTestPackage.
var MgoAddr string

// MgoSuite is a suite that deletes all content from the shared MongoDB
// server at the end of every test.
type MgoSuite struct{}

// MgoSessionSuite is a suite that supplies a connection to the shared
// MongoDB server.
type MgoSessionSuite struct {
	MgoSuite
	MgoSession *mgo.Session
}

// StartMgoServer starts a MongoDB server in a temporary directory.
// It panics if it encounters an error.
func StartMgoServer() (server *exec.Cmd, dbdir string) {
	dbdir, err := ioutil.TempDir("", "test-mgo")
	if err != nil {
		panic(fmt.Errorf("cannot create temporary directory: %v", err))
	}
	mgoport :=  strconv.Itoa(FindTCPPort())
	mgoargs := []string{
		"--dbpath", dbdir,
		"--bind_ip", "localhost",
		"--port", mgoport,
		"--nssize", "1",
		"--noprealloc",
		"--smallfiles",
		"--nojournal",
	}
	server = exec.Command("mongod", mgoargs...)
	err = server.Start()
	if err != nil {
		os.RemoveAll(dbdir)
		panic(fmt.Errorf("cannot start MongoDB server: %v", err))
	}
	MgoAddr = "localhost:" + mgoport
	return server, dbdir
}

// MgoTestPackage should be called to register the tests for any package that
// requires a MongoDB server.
func MgoTestPackage(t *stdtesting.T) {
	server, dbdir := StartMgoServer()
	defer func() {
		server.Process.Kill()
		server.Process.Wait()
		os.RemoveAll(dbdir)
	}()
	TestingT(t)
}

func (s MgoSuite) SetUpSuite(c *C) {
	if MgoAddr == "" {
		panic("MgoSuite tests must be run with MgoTestPackage")
	}
}

// MgoDial returns a new connection to the shared MongoDB server.
func MgoDial() *mgo.Session {
	session, err := mgo.Dial(MgoAddr)
	if err != nil {
		panic(err)
	}
	return session
}

// MgoReset deletes all content from the shared MongoDB server.
func MgoReset() {
	session := MgoDial()
	defer session.Close()
	dbnames, err := session.DatabaseNames()
	if err != nil {
		panic("Cannot get MongoDB database names")
	}
	for _, name := range dbnames {
		switch name {
		case "admin", "local", "config":
		default:
			err = session.DB(name).DropDatabase()
			if err != nil {
				panic(fmt.Errorf("Cannot drop MongoDB database %v: %v", name, err))
			}
		}
	}
}

func (s MgoSuite) TearDownTest(c *C) {
	MgoReset()
}

func (s *MgoSessionSuite) SetUpSuite(c *C) {
	s.MgoSuite.SetUpSuite(c)
	s.MgoSession = MgoDial()
}

func (s *MgoSessionSuite) TearDownSuite(c *C) {
	s.MgoSession.Close()
}
