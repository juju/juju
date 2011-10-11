package store_test

import (
	"bytes"
	"exec"
	. "launchpad.net/gocheck"
	"launchpad.net/mgo"
	"os"
	"time"
)

// ----------------------------------------------------------------------------
// The mgo test suite

type cLogger C

func (c *cLogger) Output(calldepth int, s string) os.Error {
	ns := time.Nanoseconds()
	t := float64(ns%100e9) / 1e9
	((*C)(c)).Logf("%.05f %s", t, s)
	return nil
}

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
		"--noprealloc", "--smallfiles",
	}
	s.server = exec.Command("mongod", args...)
	s.server.Stdout = &s.output
	s.server.Stderr = &s.output
	err := s.server.Start()
	if err != nil {
		panic(err)
	}
}

func (s *MgoSuite) TearDownSuite(c *C) {
	s.server.Process.Kill()
	s.server.Process.Wait(0)
}

func (s *MgoSuite) SetUpTest(c *C) {
	err := DropAll("localhost:50017")
	if err != nil {
		panic(err)
	}
	mgo.SetLogger((*cLogger)(c))
	mgo.ResetStats()
	s.Addr = "127.0.0.1:50017"
	s.Session, err = mgo.Mongo(s.Addr)
	if err != nil {
		panic(err)
	}
}

func (s *MgoSuite) TearDownTest(c *C) {
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
		time.Sleep(5e8)
	}
}

func DropAll(mongourl string) (err os.Error) {
	session, err := mgo.Mongo(mongourl)
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
