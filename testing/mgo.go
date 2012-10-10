package testing

import (
	"fmt"
	"io/ioutil"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/log"
	"net"
	"os"
	"os/exec"
	"strconv"
	stdtesting "testing"
	"time"
)

var (
	// MgoAddr holds the address of the shared MongoDB server set up by
	// MgoTestPackage.
	MgoAddr string

	// mgoServer holds the running MongoDB command.
	mgoServer *exec.Cmd

	// mgoDir holds the directory that MongoDB is running in.
	mgoDir string
)

// MgoSuite is a suite that deletes all content from the shared MongoDB
// server at the end of every test and supplies a connection to the shared
// MongoDB server.
type MgoSuite struct {
	Session *mgo.Session
}

// startMgoServer starts a MongoDB server in a temporary directory.
// It panics if it encounters an error.
func startMgoServer() error {
	dbdir, err := ioutil.TempDir("", "test-mgo")
	if err != nil {
		return err
	}
	mgoport := strconv.Itoa(FindTCPPort())
	mgoargs := []string{
		"--auth",
		"--dbpath", dbdir,
		"--bind_ip", "localhost",
		"--port", mgoport,
		"--nssize", "1",
		"--noprealloc",
		"--smallfiles",
		"--nojournal",
	}
	server := exec.Command("mongod", mgoargs...)
	if err := server.Start(); err != nil {
		os.RemoveAll(dbdir)
		return err
	}
	MgoAddr = "localhost:" + mgoport
	mgoServer = server
	mgoDir = dbdir
	return nil
}

func destroyMgoServer() {
	if mgoServer != nil {
		mgoServer.Process.Kill()
		mgoServer.Process.Wait()
		os.RemoveAll(mgoDir)
		MgoAddr, mgoServer, mgoDir = "", nil, ""
	}
}

// MgoTestPackage should be called to register the tests for any package that
// requires a MongoDB server.
func MgoTestPackage(t *stdtesting.T) {
	if err := startMgoServer(); err != nil {
		t.Fatal(err)
	}
	defer destroyMgoServer()
	TestingT(t)
}

func (s *MgoSuite) SetUpSuite(c *C) {
	if MgoAddr == "" {
		panic("MgoSuite tests must be run with MgoTestPackage")
	}
	mgo.SetStats(true)
}

func (s *MgoSuite) TearDownSuite(c *C) {}

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
	if isUnauthorized(err) {
		// If we've got an unauthorized access error, we're
		// locked out of the database.  We restart it to regain
		// access.  This should only happen when tests fail.
		destroyMgoServer()
		log.Printf("testing: restarting MongoDB server after unauthorized access")
		if err := startMgoServer(); err != nil {
			panic(err)
		}
		return
	}
	if err != nil {
		panic(err)
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

func isUnauthorized(err error) bool {
	if err, ok := err.(*mgo.QueryError); ok {
		if err.Code == 10057 || err.Message == "need to login" {
			return true
		}
	}
	return false
}

func (s *MgoSuite) TearDownTest(c *C) {
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

// FindTCPPort finds an unused TCP port and returns it.
// Use of this function has an inherent race condition - another
// process may claim the port before we try to use it.
// We hope that the probability is small enough during
// testing to be negligible.
func FindTCPPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
