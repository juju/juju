// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"bytes"
	"os/exec"
	"time"

	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
)

// ----------------------------------------------------------------------------
// The mgo test suite

type MgoSuite struct {
	Addr    string
	Session *mgo.Session
	output  bytes.Buffer
	server  *exec.Cmd
}

func (s *MgoSuite) SetUpSuite(c *C) {
	mgo.SetDebug(true)
	mgo.SetStats(true)
	dbdir := c.MkDir()
	args := []string{
		"--dbpath", dbdir,
		"--bind_ip", "127.0.0.1",
		"--port", "50017",
		"--nssize", "1",
		"--noprealloc",
		"--smallfiles",
		"--nojournal",
	}
	s.server = exec.Command("mongod", args...)
	s.server.Stdout = &s.output
	s.server.Stderr = &s.output
	err := s.server.Start()
	c.Assert(err, IsNil)
}

func (s *MgoSuite) TearDownSuite(c *C) {
	s.server.Process.Kill()
	s.server.Process.Wait()
}

func (s *MgoSuite) SetUpTest(c *C) {
	err := DropAll("localhost:50017")
	c.Assert(err, IsNil)
	mgo.SetLogger(c)
	mgo.ResetStats()
	s.Addr = "127.0.0.1:50017"
	s.Session, err = mgo.Dial(s.Addr)
	c.Assert(err, IsNil)
}

func (s *MgoSuite) TearDownTest(c *C) {
	if s.Session != nil {
		s.Session.Close()
	}
	for i := 0; ; i++ {
		stats := mgo.GetStats()
		if stats.SocketsInUse == 0 && stats.SocketsAlive == 0 {
			break
		}
		if i == 100 {
			// We wait up to 10s for all workers to finish up
			c.Fatal("Test left sockets in a dirty state")
		}
		c.Logf("Waiting for sockets to die: %d in use, %d alive", stats.SocketsInUse, stats.SocketsAlive)
		time.Sleep(100 * time.Millisecond)
	}
}

func DropAll(mongourl string) (err error) {
	session, err := mgo.Dial(mongourl)
	if err != nil {
		return err
	}
	defer session.Close()

	names, err := session.DatabaseNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		switch name {
		case "admin", "local", "config":
		default:
			err = session.DB(name).DropDatabase()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
