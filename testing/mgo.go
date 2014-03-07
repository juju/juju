// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	stdtesting "testing"
	"time"

	"labix.org/v2/mgo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
)

var (
	// MgoServer is a shared mongo server used by tests.
	MgoServer = &MgoInstance{ssl: true}
)

type MgoInstance struct {
	// addr holds the address of the MongoDB server
	addr string

	// MgoPort holds the port of the MongoDB server.
	port int

	// server holds the running MongoDB command.
	server *exec.Cmd

	// exited receives a value when the mongodb server exits.
	exited <-chan struct{}

	// dir holds the directory that MongoDB is running in.
	dir string

	// ssl determines whether the MongoDB server will use TLS
	ssl bool

	// Params is a list of additional parameters that will be passed to
	// the mongod application
	Params []string
}

// Addr returns the address of the MongoDB server.
func (m *MgoInstance) Addr() string {
	return m.addr
}

// Port returns the port of the MongoDB server.
func (m *MgoInstance) Port() int {
	return m.port
}

// We specify a timeout to mgo.Dial, to prevent
// mongod failures hanging the tests.
const mgoDialTimeout = 15 * time.Second

// MgoSuite is a suite that deletes all content from the shared MongoDB
// server at the end of every test and supplies a connection to the shared
// MongoDB server.
type MgoSuite struct {
	Session *mgo.Session
}

// Start starts a MongoDB server in a temporary directory.
func (inst *MgoInstance) Start(ssl bool) error {
	dbdir, err := ioutil.TempDir("", "test-mgo")
	if err != nil {
		return err
	}

	// give them all the same keyfile so they can talk appropriately
	keyFilePath := filepath.Join(dbdir, "keyfile")
	err = ioutil.WriteFile(keyFilePath, []byte("not very secret"), 0600)
	if err != nil {
		return fmt.Errorf("cannot write key file: %v", err)
	}

	pemPath := filepath.Join(dbdir, "server.pem")
	err = ioutil.WriteFile(pemPath, []byte(ServerCert+ServerKey), 0600)
	if err != nil {
		return fmt.Errorf("cannot write cert/key PEM: %v", err)
	}
	inst.port = FindTCPPort()
	inst.addr = fmt.Sprintf("localhost:%d", inst.port)
	inst.dir = dbdir
	inst.ssl = ssl
	if err := inst.run(); err != nil {
		inst.addr = ""
		inst.port = 0
		os.RemoveAll(inst.dir)
		inst.dir = ""
	}
	return err
}

// run runs the MongoDB server at the
// address and directory already configured.
func (inst *MgoInstance) run() error {
	if inst.server != nil {
		panic("mongo server is already running")
	}

	mgoport := strconv.Itoa(inst.port)
	mgoargs := []string{
		"--auth",
		"--dbpath", inst.dir,
		"--port", mgoport,
		"--nssize", "1",
		"--noprealloc",
		"--smallfiles",
		"--nojournal",
		"--nounixsocket",
		"--oplogSize", "10",
		"--keyFile", filepath.Join(inst.dir, "keyfile"),
	}
	if inst.ssl {
		mgoargs = append(mgoargs,
			"--sslOnNormalPorts",
			"--sslPEMKeyFile", filepath.Join(inst.dir, "server.pem"),
			"--sslPEMKeyPassword", "ignored")
	}
	if inst.Params != nil {
		mgoargs = append(mgoargs, inst.Params...)
	}
	server := exec.Command("mongod", mgoargs...)
	out, err := server.StdoutPipe()
	if err != nil {
		return err
	}
	server.Stderr = server.Stdout
	exited := make(chan struct{})
	started := make(chan struct{})
	go func() {
		<-started
		lines := readLines(out, 20)
		err := server.Wait()
		exitErr, _ := err.(*exec.ExitError)
		if err == nil || exitErr != nil && exitErr.Exited() {
			// mongodb has exited without being killed, so print the
			// last few lines of its log output.
			for _, line := range lines {
				log.Infof("mongod: %s", line)
			}
		}
		close(exited)
	}()
	inst.exited = exited
	err = server.Start()
	close(started)
	if err != nil {
		return err
	}
	inst.server = server

	return nil
}

func (inst *MgoInstance) kill() {
	inst.server.Process.Kill()
	<-inst.exited
	inst.server = nil
	inst.exited = nil
}

func (inst *MgoInstance) Destroy() {
	if inst.server != nil {
		inst.kill()
		os.RemoveAll(inst.dir)
		inst.addr, inst.dir = "", ""
	}
}

// Restart restarts the mongo server, useful for
// testing what happens when a state server goes down.
func (inst *MgoInstance) Restart() {
	inst.kill()
	if err := inst.Start(inst.ssl); err != nil {
		panic(err)
	}
}

// MgoTestPackage should be called to register the tests for any package that
// requires a MongoDB server.
func MgoTestPackage(t *stdtesting.T) {
	MgoTestPackageSsl(t, true)
}

func MgoTestPackageSsl(t *stdtesting.T, ssl bool) {
	if err := MgoServer.Start(ssl); err != nil {
		t.Fatal(err)
	}
	defer MgoServer.Destroy()
	gc.TestingT(t)
}

func (s *MgoSuite) SetUpSuite(c *gc.C) {
	if MgoServer.addr == "" {
		panic("MgoSuite tests must be run with MgoTestPackage")
	}
	mgo.SetStats(true)
	// Make tests that use password authentication faster.
	utils.FastInsecureHash = true
}

// readLines reads lines from the given reader and returns
// the last n non-empty lines, ignoring empty lines.
func readLines(r io.Reader, n int) []string {
	br := bufio.NewReader(r)
	lines := make([]string, n)
	i := 0
	for {
		line, err := br.ReadString('\n')
		if line = strings.TrimRight(line, "\n"); line != "" {
			lines[i%n] = line
			i++
		}
		if err != nil {
			break
		}
	}
	final := make([]string, 0, n+1)
	if i > n {
		final = append(final, fmt.Sprintf("[%d lines omitted]", i-n))
	}
	for j := 0; j < n; j++ {
		if line := lines[(j+i)%n]; line != "" {
			final = append(final, line)
		}
	}
	return final
}

func (s *MgoSuite) TearDownSuite(c *gc.C) {
	utils.FastInsecureHash = false
}

// MustDial returns a new connection to the MongoDB server, and panics on
// errors.
func (inst *MgoInstance) MustDial() *mgo.Session {
	s, err := inst.dial(false)
	if err != nil {
		panic(err)
	}
	return s
}

// Dial returns a new connection to the MongoDB server.
func (inst *MgoInstance) Dial() (*mgo.Session, error) {
	return inst.dial(false)
}

// DialDirect returns a new direct connection to the shared MongoDB server. This
// must be used if you're connecting to a replicaset that hasn't been initiated
// yet.
func (inst *MgoInstance) DialDirect() (*mgo.Session, error) {
	return inst.dial(true)
}

// MustDialDirect works like DialDirect, but panics on errors.
func (inst *MgoInstance) MustDialDirect() *mgo.Session {
	session, err := inst.dial(true)
	if err != nil {
		panic(err)
	}
	return session
}

func (inst *MgoInstance) dial(direct bool) (*mgo.Session, error) {
	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert([]byte(CACert))
	if err != nil {
		return nil, err
	}
	pool.AddCert(xcert)
	tlsConfig := &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	session, err := mgo.DialWithInfo(&mgo.DialInfo{
		Direct: direct,
		Addrs:  []string{inst.addr},
		Dial: func(addr net.Addr) (net.Conn, error) {
			return tls.Dial("tcp", addr.String(), tlsConfig)
		},
		Timeout: mgoDialTimeout,
	})
	return session, err
}

func (s *MgoSuite) SetUpTest(c *gc.C) {
	mgo.ResetStats()
	s.Session = MgoServer.MustDial()
}

// Reset deletes all content from the MongoDB server and panics if it encounters
// errors.
func (inst *MgoInstance) Reset() {
	session := inst.MustDial()
	defer session.Close()

	dbnames, ok := resetAdminPasswordAndFetchDBNames(session)
	if ok {
		log.Infof("Reset successfully reset admin password")
	} else {
		// We restart it to regain access.  This should only
		// happen when tests fail.
		log.Noticef("testing: restarting MongoDB server after unauthorized access")
		inst.Destroy()
		if err := inst.Start(inst.ssl); err != nil {
			panic(err)
		}
		return
	}
	for _, name := range dbnames {
		switch name {
		case "admin", "local", "config":
		default:
			if err := session.DB(name).DropDatabase(); err != nil {
				panic(fmt.Errorf("Cannot drop MongoDB database %v: %v", name, err))
			}
		}
	}
}

// resetAdminPasswordAndFetchDBNames logs into the database with a
// plausible password and returns all the database's db names. We need
// to try several passwords because we don't know what state the mongo
// server is in when Reset is called. If the test has set a custom
// password, we're out of luck, but if they are using
// DefaultStatePassword, we can succeed.
func resetAdminPasswordAndFetchDBNames(session *mgo.Session) ([]string, bool) {
	// First try with no password
	dbnames, err := session.DatabaseNames()
	if err == nil {
		return dbnames, true
	}
	if !isUnauthorized(err) {
		panic(err)
	}
	// Then try the two most likely passwords in turn.
	for _, password := range []string{
		DefaultMongoPassword,
		utils.UserPasswordHash(DefaultMongoPassword, utils.CompatSalt),
	} {
		admin := session.DB("admin")
		if err := admin.Login("admin", password); err != nil {
			log.Infof("failed to log in with password %q", password)
			continue
		}
		dbnames, err := session.DatabaseNames()
		if err == nil {
			if err := admin.RemoveUser("admin"); err != nil {
				panic(err)
			}
			return dbnames, true
		}
		if !isUnauthorized(err) {
			panic(err)
		}
		log.Infof("unauthorized access when getting database names; password %q", password)
	}
	return nil, false
}

// isUnauthorized is a copy of the same function in state/open.go.
func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string.
	if err.Error() == "auth fails" {
		return true
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}

func (s *MgoSuite) TearDownTest(c *gc.C) {
	MgoServer.Reset()
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
