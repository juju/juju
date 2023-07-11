// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

type executeArgs struct {
	command string
	args    []string
	dir     string
}

type executeResults struct {
	runError error
	exitCode int

	stdout, stderr []byte
}

func (res executeResults) String() string {
	var str string
	str += fmt.Sprintf("runError: %v\n", res.runError)
	str += fmt.Sprintf("exitCode: %d\n", res.exitCode)
	str += fmt.Sprintf("stdout: %s\n", res.stdout)
	str += fmt.Sprintf("stderr: %s", res.stderr)
	return str
}

func execute(args executeArgs) (res executeResults) {
	cmd := exec.Command(args.command, args.args...)
	cmd.Dir = args.dir

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	err := cmd.Run()
	res.runError = err
	if e := (&exec.ExitError{}); errors.As(err, &e) {
		res.exitCode = e.ProcessState.ExitCode()
	}
	res.stdout, res.stderr = stdout.Bytes(), stderr.Bytes()
	return
}

func handleExecuteError(res executeResults) {
	if res.exitCode > 0 {
		stderrf("command failed with exit code %d\n", res.exitCode)
	}
	if err := res.runError; err != nil {
		stderrf("stderr: %s\n", res.stderr)
		panic(err)
	}
}
