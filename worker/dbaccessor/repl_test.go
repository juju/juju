// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"io/ioutil"
	"os"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type REPLSuite struct{}

var _ = gc.Suite(&REPLSuite{})

func (s *REPLSuite) TestREPL(c *gc.C) {
	//filePath, done := s.makeTempSocket(c)
	//defer done()

	repl, err := newREPL("/tmp/socket", nil, func(e error) bool {
		return false
	}, clock.WallClock, fakeLogger{})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckKill(c, repl)
}

func (s *REPLSuite) makeTempSocket(c *gc.C) (string, func()) {
	file, err := ioutil.TempFile("/tmp", "socket")
	if err != nil {
		c.Fatal(err)
	}
	return file.Name(), func() {
		_ = os.Remove(file.Name())
	}
}

type fakeLogger struct{}

func (fakeLogger) Errorf(_ string, _ ...interface{})   {}
func (fakeLogger) Infof(_ string, _ ...interface{})    {}
func (fakeLogger) Warningf(_ string, _ ...interface{}) {}
func (fakeLogger) Tracef(_ string, _ ...interface{})   {}
