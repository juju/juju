// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/juju/utils/v4/exec"
)

func TestUnitApplication(t *testing.T) {
	tests := []struct {
		name     string
		unitName string
		want     string
		wantErr  bool
	}{
		{name: "valid", unitName: "mysql/3", want: "mysql"},
		{name: "hyphenated application", unitName: "my-app/12", want: "my-app"},
		{name: "missing slash", unitName: "mysql", wantErr: true},
		{name: "missing number", unitName: "mysql/", wantErr: true},
		{name: "non numeric unit number", unitName: "mysql/not-a-number", wantErr: true},
		{name: "extra slash", unitName: "mysql/0/1", wantErr: true},
	}

	for _, test := range tests {
		got, err := unitApplication(test.unitName)
		if test.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", test.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", test.name, err)
		}
		if got != test.want {
			t.Fatalf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}

func TestWriteError(t *testing.T) {
	var buf bytes.Buffer
	writeError(&buf, errors.New("boom"))
	if got, want := buf.String(), "ERROR boom\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHookToolMainRetriesWithStdin(t *testing.T) {
	fake := &fakeRPCClient{}
	restoreDialer := swapRPCDialer(fake)
	defer restoreDialer()
	t.Setenv("JUJU_CONTEXT_ID", "context-id")
	t.Setenv("JUJU_AGENT_SOCKET_NETWORK", "unix")
	t.Setenv("JUJU_AGENT_SOCKET_ADDRESS", "/tmp/jujuc.socket")

	workingDir := t.TempDir()
	restoreWd := chdir(t, workingDir)
	defer restoreWd()

	stdinFile, err := writeTempFile(t, "stdin payload")
	if err != nil {
		t.Fatalf("stdin file: %v", err)
	}
	defer func() { _ = stdinFile.Close() }()

	restoreStdio, stdoutReader, stderrReader := swapStdio(t, stdinFile)
	restored := false
	defer func() {
		if !restored {
			restoreStdio()
		}
	}()
	defer func() {
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
	}()

	code, err := hookToolMain("relation-get", []string{"relation-get", "--format", "json"})
	if err != nil {
		t.Fatalf("hookToolMain: %v", err)
	}
	if code != 17 {
		t.Fatalf("got exit code %d, want 17", code)
	}
	restoreStdio()
	restored = true

	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if got, want := string(stdout), "tool stdout"; got != want {
		t.Fatalf("got stdout %q, want %q", got, want)
	}
	if got, want := string(stderr), "tool stderr"; got != want {
		t.Fatalf("got stderr %q, want %q", got, want)
	}

	requests := fake.Requests()
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want 2", len(requests))
	}
	if requests[0].StdinSet {
		t.Fatalf("first request unexpectedly had stdin")
	}
	if !requests[1].StdinSet {
		t.Fatalf("second request should have stdin")
	}
	if got, want := string(requests[1].Stdin), "stdin payload"; got != want {
		t.Fatalf("got stdin %q, want %q", got, want)
	}
	if got, want := requests[1].ContextId, "context-id"; got != want {
		t.Fatalf("got context id %q, want %q", got, want)
	}
	if got, want := requests[1].CommandName, "relation-get"; got != want {
		t.Fatalf("got command %q, want %q", got, want)
	}
	if got, want := requests[1].Dir, workingDir; got != want {
		t.Fatalf("got dir %q, want %q", got, want)
	}
	if got, want := requests[1].Args[0], "--format"; got != want {
		t.Fatalf("got arg[0] %q, want %q", got, want)
	}
	if got, want := requests[1].Args[1], "json"; got != want {
		t.Fatalf("got arg[1] %q, want %q", got, want)
	}
	if !fake.closed {
		t.Fatalf("expected rpc client to be closed")
	}
}

type fakeJujucServer struct {
	mu       sync.Mutex
	requests []Request
}

func (s *fakeJujucServer) Main(req Request, resp *exec.ExecResponse) error {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()

	if !req.StdinSet {
		return errors.New(ErrNoStdinStr)
	}
	resp.Code = 17
	resp.Stdout = []byte("tool stdout")
	resp.Stderr = []byte("tool stderr")
	return nil
}

func (s *fakeJujucServer) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Request, len(s.requests))
	copy(result, s.requests)
	return result
}

type fakeRPCClient struct {
	fakeJujucServer
	closed bool
}

func (c *fakeRPCClient) Call(serviceMethod string, args interface{}, reply interface{}) error {
	if serviceMethod != "Jujuc.Main" {
		return errors.New("unexpected service method")
	}
	req, ok := args.(Request)
	if !ok {
		return errors.New("unexpected request type")
	}
	resp, ok := reply.(*exec.ExecResponse)
	if !ok {
		return errors.New("unexpected reply type")
	}
	return c.Main(req, resp)
}

func (c *fakeRPCClient) Close() error {
	c.closed = true
	return nil
}

func swapRPCDialer(client rpcClient) func() {
	oldDialer := dialRPCClientFunc
	dialRPCClientFunc = func(socket socketConfig) (rpcClient, error) {
		return client, nil
	}
	return func() {
		dialRPCClientFunc = oldDialer
	}
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}
}

func writeTempFile(t *testing.T, contents string) (*os.File, error) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		return nil, err
	}
	if _, err := file.WriteString(contents); err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return file, nil
}

func swapStdio(t *testing.T, stdin *os.File) (func(), *os.File, *os.File) {
	t.Helper()
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdin = stdin
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	return func() {
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}, stdoutReader, stderrReader
}
