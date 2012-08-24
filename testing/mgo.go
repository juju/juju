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
	"time"
)

// MgoAddr holds the address of the shared MongoDB server set up by
// StartMgoServer.
var MgoAddr string

// MgoSuite is a suite that deletes all content from the shared MongoDB
// server at the end of every test and supplies a connection to the shared
// MongoDB server.
type MgoSuite struct {
	Session *mgo.Session
}

// StartMgoServer starts a MongoDB server in a temporary directory.
// It panics if it encounters an error.
func StartMgoServer() (server *exec.Cmd, dbdir string) {
	dbdir, err := ioutil.TempDir("", "test-mgo")
	if err != nil {
		panic(fmt.Errorf("cannot create temporary directory: %v", err))
	}
	mgoport := strconv.Itoa(FindTCPPort())
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

func MgoDestroy(server *exec.Cmd, dbdir string) {
	server.Process.Kill()
	server.Process.Wait()
	os.RemoveAll(dbdir)
}

// MgoTestPackage should be called to register the tests for any package that
// requires a MongoDB server.
func MgoTestPackage(t *stdtesting.T) {
	server, dbdir := StartMgoServer()
	defer MgoDestroy(server, dbdir)
	TestingT(t)
}

func (s MgoSuite) SetUpSuite(c *C) {
	if MgoAddr == "" {
		panic("MgoSuite tests must be run with MgoTestPackage")
	}
	mgo.SetStats(true)
}

// MgoDial returns a new connection to the shared MongoDB server.
func MgoDial() *mgo.Session {
	session, err := mgo.Dial(MgoAddr)
	if err != nil {
		panic(err)
	}
	return session
}

func (s *MgoSuite) SetUpTest(c *C) {
	mgo.ResetStats()
	s.Session = MgoDial()
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
	s.Session.Close()
	for i := 0; ; i++ {
		stats := mgo.GetStats()
		if stats.SocketsInUse == 0 && stats.SocketsAlive == 0 {
			break
		}
		if i == 20 {
			c.Fatal("Test left sockets in a dirty state")
		}
		c.Logf("Waiting for sockets to die: %d in use, %d alive", stats.SocketsInUse, stats.SocketsAlive)
		time.Sleep(500 * time.Millisecond)
	}
}
