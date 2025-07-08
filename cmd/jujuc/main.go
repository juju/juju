// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3/exec"

	"github.com/juju/juju/internal/debug/coveruploader"
	"github.com/juju/juju/juju/sockets"
)

var logger = loggo.GetLogger("juju.cmd.jujud")

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

const (
	// exit_err is the value that is returned when the user has run juju in an invalid way.
	exit_err = 2
	// exit_panic is the value that is returned when we exit due to an unhandled panic.
	exit_panic = 3
)

func getenv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", errors.Errorf("%s not set", name)
	}
	return value, nil
}

func getwd() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func getSocket() (sockets.Socket, error) {
	var err error
	socket := sockets.Socket{}
	socket.Address, err = getenv("JUJU_AGENT_SOCKET_ADDRESS")
	if err != nil {
		return sockets.Socket{}, err
	}
	socket.Network, err = getenv("JUJU_AGENT_SOCKET_NETWORK")
	if err != nil {
		return sockets.Socket{}, err
	}

	// If we are not connecting over tcp, no need for TLS.
	if socket.Network != "tcp" {
		return socket, nil
	}

	caCertFile, err := getenv("JUJU_AGENT_CA_CERT")
	if err != nil {
		return sockets.Socket{}, err
	}
	caCert, err := ioutil.ReadFile(caCertFile)
	if err != nil {
		return sockets.Socket{}, errors.Annotatef(err, "reading %s", caCertFile)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caCert); ok == false {
		return sockets.Socket{}, errors.Errorf("invalid ca certificate")
	}

	unitName, err := getenv("JUJU_UNIT_NAME")
	if err != nil {
		return sockets.Socket{}, err
	}
	application, err := names.UnitApplication(unitName)
	if err != nil {
		return sockets.Socket{}, errors.Trace(err)
	}
	socket.TLSConfig = &tls.Config{
		RootCAs:    rootCAs,
		ServerName: application,
	}
	return socket, nil
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

	Token string
}

var ErrNoStdinStr = "hook tool requires stdin, none supplied"

// hookToolMain uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET_ADDRESS to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
func hookToolMain(commandName string, ctx *cmd.Context, args []string) (code int, err error) {
	code = 1
	contextID, err := getenv("JUJU_CONTEXT_ID")
	if err != nil {
		return
	}
	dir, err := getwd()
	if err != nil {
		return
	}
	req := Request{
		ContextId:   contextID,
		Dir:         dir,
		CommandName: commandName,
		Args:        args[1:],
		Token:       os.Getenv("JUJU_AGENT_TOKEN"),
	}
	socket, err := getSocket()
	if err != nil {
		return
	}
	client, err := sockets.Dial(socket)
	if err != nil {
		return code, err
	}
	defer client.Close()
	var resp exec.ExecResponse
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil && err.Error() == ErrNoStdinStr {
		req.Stdin, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			err = errors.Annotate(err, "cannot read stdin")
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
			logger.Criticalf("Unhandled panic: \n%v\n%s", r, buf)
			os.Exit(exit_panic)
		}
	}()

	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(exit_err)
	}

	var code int
	commandName := filepath.Base(args[0])
	code, err = hookToolMain(commandName, ctx, args)
	if err != nil {
		cmd.WriteError(ctx.Stderr, err)
	}
	return code
}
