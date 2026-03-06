// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/internal/debug/coveruploader"
)

const (
	// ExitStatusCodeErr is the value that is returned when the user has run juju in an invalid way.
	ExitStatusCodeErr = 2
	// ExitStatusCodePanic is the value that is returned when we exit due to an unhandled panic.
	ExitStatusCodePanic = 3

	errorPrefix = "ERROR"
)

func getenv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s not set", name)
	}
	return value, nil
}

type socketConfig struct {
	Address   string
	Network   string
	TLSConfig *tls.Config
}

func getSocket() (socketConfig, error) {
	var err error
	socket := socketConfig{}
	socket.Address, err = getenv("JUJU_AGENT_SOCKET_ADDRESS")
	if err != nil {
		return socketConfig{}, err
	}
	socket.Network, err = getenv("JUJU_AGENT_SOCKET_NETWORK")
	if err != nil {
		return socketConfig{}, err
	}

	// If we are not connecting over tcp, no need for TLS.
	if socket.Network != "tcp" {
		return socket, nil
	}

	caCertFile, err := getenv("JUJU_AGENT_CA_CERT")
	if err != nil {
		return socketConfig{}, err
	}
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return socketConfig{}, fmt.Errorf("reading %s: %w", caCertFile, err)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCert); ok == false {
		return socketConfig{}, fmt.Errorf("invalid ca certificate")
	}

	unitName, err := getenv("JUJU_UNIT_NAME")
	if err != nil {
		return socketConfig{}, err
	}
	application, err := unitApplication(unitName)
	if err != nil {
		return socketConfig{}, err
	}
	socket.TLSConfig = &tls.Config{
		RootCAs:    rootCAs,
		ServerName: application,
	}
	return socket, nil
}

func unitApplication(unitName string) (string, error) {
	if strings.Count(unitName, "/") != 1 {
		return "", fmt.Errorf("%q is not a valid unit name", unitName)
	}
	i := strings.LastIndexByte(unitName, '/')
	if i <= 0 || i >= len(unitName)-1 {
		return "", fmt.Errorf("%q is not a valid unit name", unitName)
	}
	if _, err := strconv.Atoi(unitName[i+1:]); err != nil {
		return "", fmt.Errorf("%q is not a valid unit name", unitName)
	}
	return unitName[:i], nil
}

func dialRPCClient(socket socketConfig) (*rpc.Client, error) {
	var (
		conn net.Conn
		err  error
	)
	if socket.TLSConfig != nil {
		conn, err = tls.Dial(socket.Network, socket.Address, socket.TLSConfig)
	} else {
		conn, err = net.Dial(socket.Network, socket.Address)
	}
	if err != nil {
		return nil, err
	}
	return rpc.NewClient(conn), nil
}

type rpcClient interface {
	Call(serviceMethod string, args interface{}, reply interface{}) error
	Close() error
}

var dialRPCClientFunc = func(socket socketConfig) (rpcClient, error) {
	return dialRPCClient(socket)
}

func writeError(w io.Writer, err error) {
	fmt.Fprintf(w, "%s %s\n", errorPrefix, err)
}

type Request struct {
	ContextId   string
	Dir         string
	CommandName string
	Args        []string

	// StdinSet indicates whether or not the client supplied stdin. This is
	// necessary as Stdin will be nil if the client supplied stdin but it
	// is empty.
	StdinSet bool
	Stdin    []byte
}

var ErrNoStdinStr = "hook tool requires stdin, none supplied"

// hookToolMain uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET_ADDRESS to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
func hookToolMain(commandName string, args []string) (code int, err error) {
	code = 1
	contextID, err := getenv("JUJU_CONTEXT_ID")
	if err != nil {
		return
	}
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return
	}
	req := Request{
		ContextId:   contextID,
		Dir:         dir,
		CommandName: commandName,
		Args:        args[1:],
	}
	socket, err := getSocket()
	if err != nil {
		return
	}
	client, err := dialRPCClientFunc(socket)
	if err != nil {
		return code, err
	}
	defer client.Close()
	var resp exec.ExecResponse
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil && err.Error() == ErrNoStdinStr {
		req.Stdin, err = io.ReadAll(os.Stdin)
		if err != nil {
			err = fmt.Errorf("cannot read stdin: %w", err)
			return
		}
		req.StdinSet = true
		err = client.Call("Jujuc.Main", req, &resp)
	}
	if err != nil {
		return
	}
	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	return resp.Code, nil
}

func main() {
	coveruploader.Enable()
	os.Exit(Main(os.Args))
}

// Main is not redundant with main(), because it provides an entry point
// for testing with arbitrary command line arguments.
func Main(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			writeError(os.Stderr, fmt.Errorf("Unhandled panic: \n%v\n%s", r, buf))
			os.Exit(ExitStatusCodePanic)
		}
	}()

	var code int
	commandName := filepath.Base(args[0])
	code, err := hookToolMain(commandName, args)
	if err != nil {
		writeError(os.Stderr, err)
	}
	return code
}
