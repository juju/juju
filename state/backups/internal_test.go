// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups

import (
	"fmt"
	"os"
	"syscall"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&pathsSuite{})

type pathsSuite struct {
	testing.BaseSuite
}

func (s *pathsSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&getMongodPath, func() (string, error) {
		return "path/to/mongod", nil
	})
}

func (s *pathsSuite) TestPathDefaultMongoExists(c *gc.C) {
	calledWithPaths := []string{}
	osStat := func(aPath string) (os.FileInfo, error) {
		calledWithPaths = append(calledWithPaths, aPath)
		return nil, nil
	}
	mongoPath, err := getMongoToolPath("tool", osStat, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mongoPath, gc.Equals, "path/to/juju-db.tool")
	c.Assert(calledWithPaths, gc.DeepEquals, []string{"path/to/juju-db.tool"})
}

func (s *pathsSuite) TestPathNoDefaultMongo(c *gc.C) {
	calledWithPaths := []string{}
	osStat := func(aPath string) (os.FileInfo, error) {
		calledWithPaths = append(calledWithPaths, aPath)
		return nil, fmt.Errorf("sorry no mongo")
	}

	calledWithLookup := []string{}
	execLookPath := func(aLookup string) (string, error) {
		calledWithLookup = append(calledWithLookup, aLookup)
		return "/a/fake/mongo/path", nil
	}

	mongoPath, err := getMongoToolPath("tool", osStat, execLookPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mongoPath, gc.Equals, "/a/fake/mongo/path")
	c.Assert(calledWithPaths, gc.DeepEquals, []string{
		"path/to/juju-db.tool",
	})
	c.Assert(calledWithLookup, gc.DeepEquals, []string{"tool"})
}

func (s *pathsSuite) TestPathSnapMongo(c *gc.C) {
	statPaths := []string{}
	mockStat := func(path string) (os.FileInfo, error) {
		statPaths = append(statPaths, path)
		switch path {
		case "path/to/juju-db.mongodump":
			return nil, nil // nil FileInfo is okay; value isn't used
		default:
			return nil, &os.PathError{Op: "mockStat", Path: path, Err: syscall.ENOENT}
		}
	}

	path, err := getMongoToolPath("mongodump", mockStat, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "path/to/juju-db.mongodump")
	c.Assert(statPaths, gc.DeepEquals, []string{
		"path/to/juju-db.mongodump",
	})
}
