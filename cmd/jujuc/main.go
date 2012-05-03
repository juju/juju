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

func getenv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s not set", name)
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

// exitCode prints err and returns 1.
func exitCode(err error) int {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	return 1
}

// Main uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
// This function is not redundant with main, because it is exported, and can
// thus be called by testing code.
func Main(args []string) int {
	commandName := filepath.Base(args[0])
	if commandName == "jujuc" {
		fmt.Fprint(os.Stderr, Help)
		return 2
	}
	contextId, err := getenv("JUJU_CONTEXT_ID")
	if err != nil {
		return exitCode(err)
	}
	dir, err := getwd()
	if err != nil {
		return exitCode(err)
	}
	req := server.Request{
		ContextId:   contextId,
		Dir:         dir,
		CommandName: commandName,
		Args:        args[1:],
	}
	socketPath, err := getenv("JUJU_AGENT_SOCKET")
	if err != nil {
		return exitCode(err)
	}
	client, err := rpc.Dial("unix", socketPath)
	if err != nil {
		return exitCode(err)
	}
	defer client.Close()
	var resp server.Response
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil {
		return exitCode(err)
	}
	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	return resp.Code
}

func main() {
	os.Exit(Main(os.Args))
}
