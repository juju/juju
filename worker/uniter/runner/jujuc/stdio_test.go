// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing/iotest"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type StdioSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&StdioSuite{})

func (s *StdioSuite) TestStdioServer(c *gc.C) {
	stdin := strings.NewReader("abcdef")
	server := newStdioServer(c, iotest.TimeoutReader(stdin))
	assertRead(c, server, 0, []byte{}, false, "")
	assertRead(c, server, 4, []byte{}, false, "timeout")
	assertRead(c, server, 4, []byte("abcd"), false, "")
	assertRead(c, server, 3, []byte("ef"), false, "")
	assertRead(c, server, 99, []byte{}, true, "")

	stdin.Seek(0, 0)
	server = newStdioServer(c, iotest.DataErrReader(stdin))
	assertRead(c, server, 6, []byte("abcdef"), true, "")
}

func (s *StdioSuite) TestStdioClient(c *gc.C) {
	expectedParams := []*jujuc.ReadRequest{{1}, {1}, {1}}
	responses := []jujuc.ReadResponse{
		{[]byte("a"), false},
		{[]byte("b"), false},
		{[]byte("c"), true},
	}
	var calls int
	call := func(req rpc.Request, params, response interface{}) error {
		c.Assert(req, jc.DeepEquals, rpc.Request{
			"Stdio", 0, "", "ReadStdin",
		})
		c.Assert(params, jc.DeepEquals, expectedParams[calls])
		c.Assert(response, gc.FitsTypeOf, &jujuc.ReadResponse{})
		*response.(*jujuc.ReadResponse) = responses[calls]
		calls++
		return nil
	}

	client := &jujuc.StdioClient{mockRpcCaller{call}}
	stdin := client.Stdin()
	c.Assert(stdin, gc.NotNil)

	data, err := ioutil.ReadAll(iotest.OneByteReader(stdin))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, []byte("abc"))
	c.Assert(calls, gc.Equals, len(responses))
}

func (s *StdioSuite) TestStdioClientErrors(c *gc.C) {
	var response jujuc.ReadResponse
	var serverErr error
	call := func(req rpc.Request, params, out interface{}) error {
		*out.(*jujuc.ReadResponse) = response
		return serverErr
	}
	client := &jujuc.StdioClient{mockRpcCaller{call}}
	stdin := client.Stdin()

	buf := make([]byte, 1)
	response.EOF = true
	_, err := stdin.Read(buf)
	c.Assert(err, gc.Equals, io.EOF)

	serverErr = errors.New("badness")
	_, err = stdin.Read(buf)
	c.Assert(err, gc.Equals, serverErr)
}

func newStdioServer(c *gc.C, stdin io.Reader) *jujuc.StdioServer {
	root := jujuc.NewStdioServerRoot(stdin)
	c.Assert(root, gc.NotNil)
	server, err := root.Stdio("anything")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(server, gc.NotNil)
	return server
}

func assertRead(c *gc.C, server *jujuc.StdioServer, n int, expect []byte, expectEOF bool, expectErr string) {
	response, err := server.ReadStdin(jujuc.ReadRequest{n})
	if expectErr != "" {
		c.Assert(err, gc.ErrorMatches, expectErr)
	} else {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(response.Data, jc.DeepEquals, expect)
		c.Assert(response.EOF, gc.Equals, expectEOF)
	}

}

type mockRpcCaller struct {
	call func(req rpc.Request, params, response interface{}) error
}

func (c mockRpcCaller) Call(req rpc.Request, params, response interface{}) error {
	return c.call(req, params, response)
}
