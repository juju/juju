// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"io"
	"os"
	"reflect"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type instanceBrokerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&instanceBrokerSuite{})

func mockOpen(name string) (*os.File, error) {
	return os.Open(".")
}

func mockReadDirNamesInterfaces(f *os.File, n int) (names []string, err error) {
	return nil, io.EOF
}

func mockReadDirNamesNetplan(f *os.File, n int) (names []string, err error) {
	return nil, nil
}

func (s *instanceBrokerSuite) TestDefaultBridgerNetplan(c *gc.C) {
	s.PatchValue(&openFunc, mockOpen)
	s.PatchValue(&readDirFunc, mockReadDirNamesNetplan)

	bridger, err := defaultBridger()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reflect.TypeOf(bridger).Elem().Name(), gc.Equals, "netplanBridger")
}

func (s *instanceBrokerSuite) TestDefaultBridgerInterfaces(c *gc.C) {
	s.PatchValue(&openFunc, mockOpen)
	s.PatchValue(&readDirFunc, mockReadDirNamesInterfaces)

	bridger, err := defaultBridger()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reflect.TypeOf(bridger).Elem().Name(), gc.Equals, "etcNetworkInterfacesBridger")
}
