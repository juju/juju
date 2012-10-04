package main

import (
	"fmt"
	"launchpad.net/juju-core/worker/uniter/jujuc"
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

// Main uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET to ask a running unit agent
// to execute a Command on our behalf. Individual commands should be exposed
// by symlinking the command name to this executable.
// This function is not redundant with main, because it is exported, and can
// thus be called by testing code.
func Main(args []string) (code int, err error) {
	commandName := filepath.Base(args[0])
	if commandName == "jujuc" {
		fmt.Fprint(os.Stderr, Help)
		return 2, fmt.Errorf("jujuc should not be called directly")
	}
	code = 1
	contextId, err := getenv("JUJU_CONTEXT_ID")
	if err != nil {
		return
	}
	dir, err := getwd()
	if err != nil {
		return
	}
	req := jujuc.Request{
		ContextId:   contextId,
		Dir:         dir,
		CommandName: commandName,
		Args:        args[1:],
	}
	socketPath, err := getenv("JUJU_AGENT_SOCKET")
	if err != nil {
		return
	}
	client, err := rpc.Dial("unix", socketPath)
	if err != nil {
		return
	}
	defer client.Close()
	var resp jujuc.Response
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil {
		return
	}
	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	return resp.Code, nil
}

func main() {
	code, err := Main(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	os.Exit(code)
}
