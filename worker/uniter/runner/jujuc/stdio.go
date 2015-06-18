// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"io"

	"github.com/juju/juju/rpc"
)

// StdioServerRoot is a juju/rpc server root object that provides
// a "Stdio" RPC object with methods for interacting with the
// server's stdio.
type StdioServerRoot struct {
	stdin io.Reader
}

// NewStdioServerRoot returns a new StdioServerRoot with the
// specified reader used for stdin.
func NewStdioServerRoot(stdin io.Reader) *StdioServerRoot {
	return &StdioServerRoot{stdin}
}

// ReadRequest contains arguments for performing a Read operation.
type ReadRequest struct {
	// Len is the maximum number of bytes to read.
	Len int
}

// ReadResponse contains the result of a Read operation.
type ReadResponse struct {
	Data []byte
	EOF  bool
}

// Stdio returns the exported "Stdio" juju/rpc type.
func (s *StdioServerRoot) Stdio(id string) (*StdioServer, error) {
	return &StdioServer{s.stdin}, nil
}

// StdioServer provides methods for interacting with stdio.
type StdioServer struct {
	stdin io.Reader
}

// ReadStdin reads data from the server's stdin, and returns it to the client.
func (s *StdioServer) ReadStdin(arg ReadRequest) (ReadResponse, error) {
	buf := make([]byte, arg.Len)
	n, err := s.stdin.Read(buf)
	if err != nil && err != io.EOF {
		return ReadResponse{}, err
	}
	return ReadResponse{Data: buf[:n], EOF: err == io.EOF}, nil
}

// RpcCaller is an interface passed to StdioClient for interacting with
// a remote StdioServer. RpcCaller is implemented by *juju/rpc.Conn.
type RpcCaller interface {
	Call(req rpc.Request, params, response interface{}) error
}

// StdioClient wraps a juju/rpc connection to interact with a
// remote server of StdioServerRoot.
type StdioClient struct {
	Conn RpcCaller
}

// Stdin returns an io.Reader that reads from the server's stdin.
func (c *StdioClient) Stdin() io.Reader {
	return &stdioReader{c.Conn, "ReadStdin"}
}

type stdioReader struct {
	caller RpcCaller
	method string
}

// Read implements io.Reader.
func (r *stdioReader) Read(p []byte) (n int, err error) {
	var res ReadResponse
	args := ReadRequest{len(p)}
	err = r.caller.Call(rpc.Request{"Stdio", 0, "", r.method}, &args, &res)
	if err != nil {
		return 0, err
	}
	if res.EOF {
		err = io.EOF
	}
	n = copy(p, res.Data)
	return n, err
}
