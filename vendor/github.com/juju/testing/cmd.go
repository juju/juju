// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"
)

var HookChannelSize = 10

// HookCommandOutput intercepts CommandOutput to a function that passes the
// actual command and it's output back via a channel, and returns the error
// passed into this function.  It also returns a cleanup function so you can
// restore the original function
func HookCommandOutput(
	outputFunc *func(cmd *exec.Cmd) ([]byte, error), output []byte, err error) (<-chan *exec.Cmd, func()) {

	cmdChan := make(chan *exec.Cmd, HookChannelSize)
	origCommandOutput := *outputFunc
	cleanup := func() {
		close(cmdChan)
		*outputFunc = origCommandOutput
	}
	*outputFunc = func(cmd *exec.Cmd) ([]byte, error) {
		cmdChan <- cmd
		return output, err
	}
	return cmdChan, cleanup
}

const (
	// EchoQuotedArgs is a simple bash script that prints out the
	// basename of the command followed by the args as quoted strings.
	// If a ; separated list of exit codes is provided in $name.exitcodes
	// then it will return them in turn over multiple calls. If
	// $name.exitcodes does not exist, or the list runs out, return 0.
	EchoQuotedArgsUnix = `#!/bin/bash --norc
name=` + "`basename $0`" + `
argfile="$0.out"
exitcodesfile="$0.exitcodes"
printf "%s" $name | tee -a $argfile
for arg in "$@"; do
  printf " '%s'" "$arg" | tee -a $argfile
done
printf "\n" | tee -a $argfile
if [ -f $exitcodesfile ]
then
	exitcodes=$(cat $exitcodesfile)
	arr=(${exitcodes/;/ })
	echo ${arr[1]} | tee $exitcodesfile
	exit ${arr[0]}
fi
`
	EchoQuotedArgsWindows = `@echo off

setlocal enabledelayedexpansion
set list=%0
set argCount=0
set argfile=%~f0.out
set exitcodesfile=%~f0.exitcodes
for %%x in (%*) do (
   set /A argCount+=1
   set "argVec[!argCount!]=%%~x"
)
for /L %%i in (1,1,%argCount%) do set list=!list! '!argVec[%%i]!'

IF exist %exitcodesfile% (
    FOR /F "tokens=1* delims=;" %%i IN (%exitcodesfile%) DO (
        set exitcode=%%i
        IF NOT [%%j]==[] (
            echo %%j > %exitcodesfile%
        ) ELSE (
            del %exitcodesfile%
        )
    )
)

echo %list%>> %argfile%
exit /B %exitcode%
`
)

// EnvironmentPatcher is an interface that requires just one method:
// PatchEnvironment.
type EnvironmentPatcher interface {
	PatchEnvironment(name, value string)
}

// PatchExecutable creates an executable called 'execName' in a new test
// directory and that directory is added to the path.
func PatchExecutable(c *gc.C, patcher EnvironmentPatcher, execName, script string, exitCodes ...int) {
	dir := c.MkDir()
	patcher.PatchEnvironment("PATH", joinPathLists(dir, os.Getenv("PATH")))
	var filename string
	switch runtime.GOOS {
	case "windows":
		filename = filepath.Join(dir, execName+".bat")
	default:
		filename = filepath.Join(dir, execName)
	}
	err := ioutil.WriteFile(filename, []byte(script), 0755)
	c.Assert(err, gc.IsNil)

	if len(exitCodes) > 0 {
		filename := filename + ".exitcodes"
		codes := make([]string, len(exitCodes))
		for i, code := range exitCodes {
			codes[i] = strconv.Itoa(code)
		}
		s := strings.Join(codes, ";") + ";"
		err = ioutil.WriteFile(filename, []byte(s), 0644)
		c.Assert(err, gc.IsNil)
	}
}

// PatchExecutableThrowError is needed to test cases in which we expect exit
// codes from executables called from the system path
func PatchExecutableThrowError(c *gc.C, patcher EnvironmentPatcher, execName string, exitCode int) {
	switch runtime.GOOS {
	case "windows":
		script := fmt.Sprintf(`@echo off
		                       setlocal enabledelayedexpansion
                               echo failing
                               exit /b %d
                               REM see %%ERRORLEVEL%% for last exit code like $? on linux
                               `, exitCode)
		PatchExecutable(c, patcher, execName, script)
	default:
		script := fmt.Sprintf(`#!/bin/bash --norc
                               echo failing
                               exit %d
                               `, exitCode)
		PatchExecutable(c, patcher, execName, script)
	}
}

// PatchExecutableAsEchoArgs creates an executable called 'execName' in a new
// test directory and that directory is added to the path. The content of the
// script is 'EchoQuotedArgs', and the args file is removed using a cleanup
// function.
func PatchExecutableAsEchoArgs(c *gc.C, patcher EnvironmentPatcher, execName string, exitCodes ...int) {
	switch runtime.GOOS {
	case "windows":
		PatchExecutable(c, patcher, execName, EchoQuotedArgsWindows, exitCodes...)
	default:
		PatchExecutable(c, patcher, execName, EchoQuotedArgsUnix, exitCodes...)
	}
}

// AssertEchoArgs is used to check the args from an execution of a command
// that has been patched using PatchExecutable containing EchoQuotedArgs.
func AssertEchoArgs(c *gc.C, execName string, args ...string) {
	// Create expected output string
	expected := execName
	for _, arg := range args {
		expected = fmt.Sprintf("%s %s", expected, utils.ShQuote(arg))
	}
	actual := ReadEchoArgs(c, execName)
	c.Assert(actual, gc.Equals, expected)
}

// ReadEchoArgs is used to read the args from an execution of a command
// that has been patched using PatchExecutable containing EchoQuotedArgs.
func ReadEchoArgs(c *gc.C, execName string) string {
	execPath, err := exec.LookPath(execName)
	c.Assert(err, jc.ErrorIsNil)

	// Read in entire argument log file
	content, err := ioutil.ReadFile(execPath + ".out")
	c.Assert(err, jc.ErrorIsNil)
	lines := strings.Split(string(content), "\n")
	actual := strings.TrimSuffix(lines[0], "\r")

	// Write out the remaining lines for the next check
	content = []byte(strings.Join(lines[1:], "\n"))
	err = ioutil.WriteFile(execPath+".out", content, 0644) // or just call this filename somewhere, once.
	return actual
}

// PatchExecHelper is a type that helps you patch out calls to executables by
// patching out the exec.Command function that creates the exec.Cmd to call
// them. This is very similar to PatchExecutable above, except it works on
// windows exe files, is a lot easier to control stderr and stdout, doesn't
// require arcane bash and batch scripting, and lets you control both the output
// *and* test the arguments, all without requiring writing any garbage files to
// disk.
//
// PatchExecHelper *must* be embedded in your test suite in order to function.
// It adds a test to your testsuite which by default simply does nothing.  When
// the patched exec.Command function is called (returned by GetExecCommand),
// instead of running the requested executable, we call the test executable with
// -check.f to rnu only TestExecSuiteHelperProcess, which acts as a configurable
// main function.
type PatchExecHelper struct{}

// PatchExecConfig holds the arguments for PatchExecHelper.GetExecCommand.
type PatchExecConfig struct {
	// Stderr is the value you'd like written to stderr.
	Stderr string
	// Stdout is the value you'd like written to stdout.
	Stdout string
	// ExitCode controls the exit code of the patched executable.
	ExitCode int
	// Args is a channel that will be sent the args passed to the patched
	// execCommand function.  It should be a channel with a buffer equal to the
	// number of executions you expect to be run (often just 1).  Do not use an
	// unbuffered channel unless you're reading the channel from another
	// goroutine, or you will almost certainly block your tests indefinitely.
	Args chan<- []string
}

// GetExecCommand returns a function that can be used to patch out a use of
// exec.Command. See PatchExecConfig for details about the arguments.
func (PatchExecHelper) GetExecCommand(cfg PatchExecConfig) func(string, ...string) *exec.Cmd {
	// This method doesn't technically need to be a method on PatchExecHelper,
	// but serves as a reminder to embed PatchExecHelper.
	return func(command string, args ...string) *exec.Cmd {
		// We redirect the command to call the test executable, telling it to
		// run the TestExecSuiteHelperProcess test that got embedded into the
		// test suite, and pass the original args at the end of our args.
		//
		// Note that we don't need to include the suite name in check.f, because
		// even if you have more than one suite embedding PatchExecHelper, all
		// the tests have the same imlpementation, and the first instance of the
		// test to run calls os.Exit, and therefore none of the other tests will
		// run.
		cs := []string{"-check.f=TestExecSuiteHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)

		cmd.Env = append(
			// We must preserve os.Environ() on Windows,
			// or the subprocess will fail in weird and
			// wonderful ways.
			os.Environ(),
			"JUJU_WANT_HELPER_PROCESS=1",
			"JUJU_HELPER_PROCESS_STDERR="+cfg.Stderr,
			"JUJU_HELPER_PROCESS_STDOUT="+cfg.Stdout,
			fmt.Sprintf("JUJU_HELPER_PROCESS_EXITCODE=%d", cfg.ExitCode),
		)

		// Pass the args back on the arg channel. This is why the channel needs
		// to be buffered, so this won't block.
		if cfg.Args != nil {
			cfg.Args <- append([]string{command}, args...)
		}
		return cmd
	}
}

// TestExecSuiteHelperProcess is a fake test which is added to your test suite
// (because you remembered to embed PatchExecHelper in your suite, right?). It
// allows us to use the test executable as a helper process to get expected
// output for tests.  When run normally during tests, this test simply does
// nothing (and passes).  The above patched exec.Command runs the test
// executable with -check.f, it runs this test and enables the configurable
// behavior.  Because the test exits with os.Exit, no additional test output is
// written.
func (PatchExecHelper) TestExecSuiteHelperProcess(c *gc.C) {
	if os.Getenv("JUJU_WANT_HELPER_PROCESS") == "" {
		return
	}
	if stderr := os.Getenv("JUJU_HELPER_PROCESS_STDERR"); stderr != "" {
		fmt.Fprintln(os.Stderr, stderr)
	}
	if stdout := os.Getenv("JUJU_HELPER_PROCESS_STDOUT"); stdout != "" {
		fmt.Fprintln(os.Stdout, stdout)
	}
	code := os.Getenv("JUJU_HELPER_PROCESS_EXITCODE")
	if code == "" {
		os.Exit(0)
	}
	exit, err := strconv.Atoi(code)
	if err != nil {
		// This should be impossible, since we set this with an int above.
		panic(err)
	}
	os.Exit(exit)

}

// CaptureOutput runs the given function and captures anything written
// to Stderr or Stdout during f's execution.
func CaptureOutput(c *gc.C, f func()) (stdout []byte, stderr []byte) {
	dir := c.MkDir()
	stderrf, err := os.OpenFile(filepath.Join(dir, "stderr"), os.O_RDWR|os.O_CREATE, 0600)
	c.Assert(err, jc.ErrorIsNil)
	defer stderrf.Close()

	stdoutf, err := os.OpenFile(filepath.Join(dir, "stdout"), os.O_RDWR|os.O_CREATE, 0600)
	c.Assert(err, jc.ErrorIsNil)
	defer stdoutf.Close()

	// make a sub-functions so those defers go off ASAP.
	func() {
		origErr := os.Stderr
		defer func() { os.Stderr = origErr }()
		origOut := os.Stdout
		defer func() { os.Stdout = origOut }()
		os.Stderr = stderrf
		os.Stdout = stdoutf

		f()
	}()

	_, err = stderrf.Seek(0, 0)
	c.Assert(err, jc.ErrorIsNil)
	stderr, err = ioutil.ReadAll(stderrf)
	c.Assert(err, jc.ErrorIsNil)

	_, err = stdoutf.Seek(0, 0)
	c.Assert(err, jc.ErrorIsNil)
	stdout, err = ioutil.ReadAll(stdoutf)
	c.Assert(err, jc.ErrorIsNil)

	return stdout, stderr
}
