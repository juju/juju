package main

import (
	"fmt"
	"launchpad.net/juju/go/cmd/jujuc/server"
	"net/rpc"
	"os"
	"path/filepath"
)

// die prints an error and exits.
func die(err error) {
	fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
	fmt.Fprintf(os.Stderr, server.JUJUC_DOC)
	os.Exit(1)
}

// requireEnv gets an environment variable or dies.
func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		die(fmt.Errorf("%s not set", name))
	}
	return value
}

// requireWd gets an absolute path to the working directory or dies.
func requireWd() string {
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

// Main uses JUJU_CONTEXT_ID and JUJU_AGENT_SOCKET to ask a running unit agent to
// execute a Command on our behalf. This function is not redundant with main,
// because it provides an entry point for testing with arbitrary command line
// arguments. Individual commands should be exposed by symlinking the command
// name to this executable.
func Main(args []string) {
	req := server.Request{requireEnv("JUJU_CONTEXT_ID"), requireWd(), args}
	client, err := rpc.Dial("unix", requireEnv("JUJU_AGENT_SOCKET"))
	if err != nil {
		die(err)
	}
	defer client.Close()
	var resp server.Response
	err = client.Call("Jujuc.Main", req, &resp)
	if err != nil {
		die(err)
	}
	os.Stdout.Write([]byte(resp.Stdout))
	os.Stderr.Write([]byte(resp.Stderr))
	os.Exit(resp.Code)
}

func main() {
	Main(os.Args)
}
