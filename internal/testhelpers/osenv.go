// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testhelpers

import (
	"os"
	"runtime"
	"strings"

	"github.com/juju/tc"
)

// OsEnvSuite isolates the tests from the underlaying system environment.
// Environment variables are reset in SetUpTest and restored in TearDownTest.
type OsEnvSuite struct {
	oldEnvironment map[string]string
}

// windowsVariables is a whitelist of windows environment variables
// that will be retained if found. Some of these variables are needed
// by standard go packages (such as os.TempDir()), as well as powershell
var windowsVariables = []string{
	"ALLUSERSPROFILE",
	"APPDATA",
	"CommonProgramFiles",
	"CommonProgramFiles(x86)",
	"CommonProgramW6432",
	"COMPUTERNAME",
	"ComSpec",
	"FP_NO_HOST_CHECK",
	"HOMEDRIVE",
	"HOMEPATH",
	"LOCALAPPDATA",
	"LOGONSERVER",
	"NUMBER_OF_PROCESSORS",
	"OS",
	"Path",
	"PATHEXT",
	"PROCESSOR_ARCHITECTURE",
	"PROCESSOR_IDENTIFIER",
	"PROCESSOR_LEVEL",
	"PROCESSOR_REVISION",
	"ProgramData",
	"ProgramFiles",
	"ProgramFiles(x86)",
	"ProgramW6432",
	"PROMPT",
	"PSModulePath",
	"PUBLIC",
	"SESSIONNAME",
	"SystemDrive",
	"SystemRoot",
	"TEMP",
	"TMP",
	"USERDOMAIN",
	"USERDOMAIN_ROAMINGPROFILE",
	"USERNAME",
	"USERPROFILE",
	"windir",
}

// testingVariables is a whitelist of environment variables
// used to control Juju tests, that will be retained if found.
var testingVariables = []string{
	"JUJU_MONGOD",
	"JUJU_TEST_VERBOSE",
	"JUJU_SQL_OUTPUT",
}

func (s *OsEnvSuite) setEnviron() {
	var isWhitelisted func(string) bool
	switch runtime.GOOS {
	case "windows":
		// Lowercase variable names for comparison as they are case
		// insenstive on windows. Fancy folding not required for ascii.
		lowerEnv := make(map[string]struct{},
			len(windowsVariables)+len(testingVariables))
		for _, envVar := range windowsVariables {
			lowerEnv[strings.ToLower(envVar)] = struct{}{}
		}
		for _, envVar := range testingVariables {
			lowerEnv[strings.ToLower(envVar)] = struct{}{}
		}
		isWhitelisted = func(envVar string) bool {
			_, ok := lowerEnv[strings.ToLower(envVar)]
			return ok
		}
	default:
		isWhitelisted = func(envVar string) bool {
			for _, testingVar := range testingVariables {
				if testingVar == envVar {
					return true
				}
			}
			return false
		}
	}
	for envVar, value := range s.oldEnvironment {
		if isWhitelisted(envVar) {
			os.Setenv(envVar, value)
		}
	}
}

// osDependendClearenv will clear the environment, and based on platform, will repopulate
// with whitelisted values previously saved in s.oldEnvironment
func (s *OsEnvSuite) osDependendClearenv() {
	os.Clearenv()
	// Restore any platform required or juju testing variables.
	s.setEnviron()
}

func (s *OsEnvSuite) SetUpSuite(c *tc.C) {
	s.oldEnvironment = make(map[string]string)
	for _, envvar := range os.Environ() {
		parts := strings.SplitN(envvar, "=", 2)
		s.oldEnvironment[parts[0]] = parts[1]
	}
	s.osDependendClearenv()
}

func (s *OsEnvSuite) TearDownSuite(c *tc.C) {
	os.Clearenv()
	for name, value := range s.oldEnvironment {
		os.Setenv(name, value)
	}
}

func (s *OsEnvSuite) SetUpTest(c *tc.C) {
	s.osDependendClearenv()
}

func (s *OsEnvSuite) TearDownTest(c *tc.C) {
}
