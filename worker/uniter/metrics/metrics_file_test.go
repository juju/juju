// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics

import (
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type metricFileSuite struct {
	spoolDir string
}

var _ = gc.Suite(&metricFileSuite{})

func (s *metricFileSuite) SetUpTest(c *gc.C) {
	s.spoolDir = c.MkDir()
}

func cleanupFile(f *metricFile) {
	if f != nil {
		f.File.Close()
	}
}

func (s *metricFileSuite) TestRenameOnClose(c *gc.C) {
	fileName := filepath.Join(s.spoolDir, "foo")
	mf, err := createMetricFile(fileName)
	c.Assert(err, gc.IsNil)

	_, err = io.CopyN(mf, rand.Reader, 78666)
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(fileName)
	c.Assert(os.IsNotExist(err), jc.IsTrue)

	err = mf.Close()
	c.Assert(err, gc.IsNil)

	st, err := os.Stat(fileName)
	c.Assert(err, gc.IsNil)
	c.Assert(st.Size(), gc.Equals, int64(78666))
}

func (s *metricFileSuite) TestContention(c *gc.C) {
	fileName := filepath.Join(s.spoolDir, "foo")
	mf1, err := createMetricFile(fileName)
	c.Assert(err, gc.IsNil)
	mf2, err := createMetricFile(fileName)
	c.Assert(err, gc.IsNil)

	_, err = fmt.Fprint(mf1, "emacs")
	c.Assert(err, gc.IsNil)
	_, err = fmt.Fprint(mf2, "vi")
	c.Assert(err, gc.IsNil)

	_, err = os.Stat(fileName)
	c.Assert(os.IsNotExist(err), jc.IsTrue)

	err = mf2.Close()
	c.Assert(err, gc.IsNil)
	err = mf1.Close()
	c.Assert(err, gc.NotNil)

	st, err := os.Stat(fileName)
	c.Assert(err, gc.IsNil)
	c.Assert(st.Size(), gc.Equals, int64(2))
	contents, err := ioutil.ReadFile(fileName)
	c.Assert(err, gc.IsNil)
	c.Assert(contents, gc.DeepEquals, []byte("vi"))
}
