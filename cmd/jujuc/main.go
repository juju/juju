package main

import (
	"fmt"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"net/rpc"
	"os"
	"path/filepath"
)

var Help = `
The jujuc command forwards invocations over RPC for execution by the juju
unit agent. It expects to be called via a symlink named for the desired
remote command, and expects JUJU_AGENT_SOCKET and JUJU_CONTEXT_ID be set
in its environment.
`[1:]

// die prints an error and exits.
func die(err error) {
	fmt.Fprintf(os.Stderr, Help)
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func getenv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		die(fmt.Errorf("%s not set", name))
	}
	return value
}

func getwd() string {
	dir, err := os.Getwd()
	if err != nil {
		die(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		die(err)
	}
	return abs
}

// Main uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
// This function is not redundant with main, because it is exported, and can
// thus be called by testing code.
func Main(args []string) {
	req := server.Request{
		ContextId:   getenv("JUJU_CONTEXT_ID"),
		Dir:         getwd(),
		CommandName: filepath.Base(args[0]),
		Args:        args[1:],
	}
	client, err := rpc.Dial("unix", getenv("JUJU_AGENT_SOCKET"))
	if err != nil {
		die(err)
	}
	defer client.Close()
	var resp server.Response
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil {
		die(err)
	}
	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	os.Exit(resp.Code)
}

func main() {
	Main(os.Args)
}
