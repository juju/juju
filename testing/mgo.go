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
	// MgoAddr holds the address of the shared MongoDB server set up by
	// MgoTestPackage.
	MgoAddr string

	// MgoPort holds the port used by the shared MongoDB server.
	MgoPort int

	// mgoServer holds the running MongoDB command.
	mgoServer *exec.Cmd

	// mgoExited receives a value when the mongodb server exits.
	mgoExited <-chan struct{}

	// mgoDir holds the directory that MongoDB is running in.
	mgoDir string
)

// We specify a timeout to mgo.Dial, to prevent
// mongod failures hanging the tests.
const mgoDialTimeout = 5 * time.Second

// MgoSuite is a suite that deletes all content from the shared MongoDB
// server at the end of every test and supplies a connection to the shared
// MongoDB server.
type MgoSuite struct {
	Session *mgo.Session
}

// startMgoServer starts a MongoDB server in a temporary directory.
func startMgoServer() error {
	dbdir, err := ioutil.TempDir("", "test-mgo")
	if err != nil {
		return err
	}
	pemPath := filepath.Join(dbdir, "server.pem")
	err = ioutil.WriteFile(pemPath, []byte(ServerCert+ServerKey), 0600)
	if err != nil {
		return fmt.Errorf("cannot write cert/key PEM: %v", err)
	}
	MgoPort = FindTCPPort()
	MgoAddr = fmt.Sprintf("localhost:%d", MgoPort)
	mgoDir = dbdir
	if err := runMgoServer(); err != nil {
		MgoAddr = ""
		MgoPort = 0
		os.RemoveAll(mgoDir)
		mgoDir = ""
		return err
	}
	return nil
}

// runMgoServer runs the MongoDB server at the
// address and directory already configured.
func runMgoServer() error {
	if mgoServer != nil {
		panic("mongo server is already running")
	}
	mgoport := strconv.Itoa(MgoPort)
	mgoargs := []string{
		"--auth",
		"--dbpath", mgoDir,
		"--sslOnNormalPorts",
		"--sslPEMKeyFile", filepath.Join(mgoDir, "server.pem"),
		"--sslPEMKeyPassword", "ignored",
		"--bind_ip", "localhost",
		"--port", mgoport,
		"--nssize", "1",
		"--noprealloc",
		"--smallfiles",
		"--nojournal",
		"--nounixsocket",
	}
	server := exec.Command("mongod", mgoargs...)
	out, err := server.StdoutPipe()
	if err != nil {
		return err
	}
	server.Stderr = server.Stdout
	exited := make(chan struct{})
	go func() {
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
	mgoExited = exited
	if err := server.Start(); err != nil {
		return err
	}
	mgoServer = server
	return nil
}

func mgoKill() {
	mgoServer.Process.Kill()
	<-mgoExited
	mgoServer = nil
	mgoExited = nil
}

func destroyMgoServer() {
	if mgoServer != nil {
		mgoKill()
		os.RemoveAll(mgoDir)
		MgoAddr, mgoDir = "", ""
	}
}

// MgoRestart restarts the mongo server, useful for
// testing what happens when a state server goes down.
func MgoRestart() {
	mgoKill()
	if err := startMgoServer(); err != nil {
		panic(err)
	}
}

// MgoTestPackage should be called to register the tests for any package that
// requires a MongoDB server.
func MgoTestPackage(t *stdtesting.T) {
	if err := startMgoServer(); err != nil {
		t.Fatal(err)
	}
	defer destroyMgoServer()
	gc.TestingT(t)
}

func (s *MgoSuite) SetUpSuite(c *gc.C) {
	if MgoAddr == "" {
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

// MgoDial returns a new connection to the shared MongoDB server.
func MgoDial() *mgo.Session {
	pool := x509.NewCertPool()
	xcert, err := cert.ParseCert([]byte(CACert))
	if err != nil {
		panic(err)
	}
	pool.AddCert(xcert)
	tlsConfig := &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	session, err := mgo.DialWithInfo(&mgo.DialInfo{
		Addrs: []string{MgoAddr},
		Dial: func(addr net.Addr) (net.Conn, error) {
			return tls.Dial("tcp", addr.String(), tlsConfig)
		},
		Timeout: mgoDialTimeout,
	})
	if err != nil {
		panic(err)
	}
	return session
}

func (s *MgoSuite) SetUpTest(c *gc.C) {
	mgo.ResetStats()
	s.Session = MgoDial()
}

// MgoReset deletes all content from the shared MongoDB server.
func MgoReset() {
	session := MgoDial()
	defer session.Close()

	dbnames, ok := resetAdminPasswordAndFetchDBNames(session)
	if ok {
		log.Infof("MgoReset successfully reset admin password")
	} else {
		// We restart it to regain access.  This should only
		// happen when tests fail.
		log.Noticef("testing: restarting MongoDB server after unauthorized access")
		destroyMgoServer()
		if err := startMgoServer(); err != nil {
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
// server is in when MgoReset is called. If the test has set a custom
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
