// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/network"
	"github.com/juju/juju/wrench"
)

var logger = loggo.GetLogger("juju.testing")

// JujuOSEnvSuite isolates the tests from Juju environment variables.
// This is intended to be only used by existing suites, usually embedded in
// BaseSuite and in FakeJujuHomeSuite. Eventually the tests relying on
// JujuOSEnvSuite will be converted to use the IsolationSuite in
// github.com/juju/testing, and this suite will be removed.
// Do not use JujuOSEnvSuite when writing new tests.
type JujuOSEnvSuite struct {
	oldJujuHome         string
	oldHomeEnv          string
	oldEnvironment      map[string]string
	initialFeatureFlags string
	regKeyExisted       bool
	regEntryExisted     bool
	oldRegEntryValue    string
}

func (s *JujuOSEnvSuite) SetUpSuite(c *gc.C) {
}

func (s *JujuOSEnvSuite) TearDownSuite(c *gc.C) {
}

func (s *JujuOSEnvSuite) SetUpTest(c *gc.C) {
	s.oldEnvironment = make(map[string]string)
	for _, name := range []string{
		osenv.JujuHomeEnvKey,
		osenv.JujuEnvEnvKey,
		osenv.JujuLoggingConfigEnvKey,
		osenv.JujuFeatureFlagEnvKey,
	} {
		s.oldEnvironment[name] = os.Getenv(name)
		os.Setenv(name, "")
	}
	s.oldHomeEnv = utils.Home()
	s.oldJujuHome = osenv.SetJujuHome("")
	utils.SetHome("")

	// Update the feature flag set to be the requested initial set.
	s.setUpFeatureFlags(c)
}

func (s *JujuOSEnvSuite) TearDownTest(c *gc.C) {
	for name, value := range s.oldEnvironment {
		os.Setenv(name, value)
	}
	s.tearDownFeatureFlags(c)
	utils.SetHome(s.oldHomeEnv)
	osenv.SetJujuHome(s.oldJujuHome)
}

// SkipIfPPC64EL skips the test if the arch is PPC64EL and the
// compiler is gccgo.
func SkipIfPPC64EL(c *gc.C, bugID string) {
	if runtime.Compiler == "gccgo" &&
		arch.NormaliseArch(runtime.GOARCH) == arch.PPC64EL {
		c.Skip(fmt.Sprintf("Test disabled on PPC64EL until fixed - see bug %s", bugID))
	}
}

// SkipIfI386 skips the test if the arch is I386.
func SkipIfI386(c *gc.C, bugID string) {
	if arch.NormaliseArch(runtime.GOARCH) == arch.I386 {
		c.Skip(fmt.Sprintf("Test disabled on I386 until fixed - see bug %s", bugID))
	}
}

// SkipIfWindowsBug skips the test if the OS is Windows.
func SkipIfWindowsBug(c *gc.C, bugID string) {
	if runtime.GOOS == "windows" {
		c.Skip(fmt.Sprintf("Test disabled on Windows until fixed - see bug %s", bugID))
	}
}

// SetInitialFeatureFlags sets the feature flags to be in effect for
// the next call to SetUpTest.
func (s *JujuOSEnvSuite) SetInitialFeatureFlags(flags ...string) {
	s.initialFeatureFlags = strings.Join(flags, ",")
}

func (s *JujuOSEnvSuite) SetFeatureFlags(flag ...string) {
	flags := strings.Join(flag, ",")
	if err := os.Setenv(osenv.JujuFeatureFlagEnvKey, flags); err != nil {
		panic(err)
	}
	logger.Debugf("setting feature flags: %s", flags)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

// BaseSuite provides required functionality for all test suites
// when embedded in a gocheck suite type:
// - logger redirect
// - no outgoing network access
// - protection of user's home directory
// - scrubbing of env vars
// TODO (frankban) 2014-06-09: switch to using IsolationSuite.
// NOTE: there will be many tests that fail when you try to change
// to the IsolationSuite that rely on external things in PATH.
type BaseSuite struct {
	testing.CleanupSuite
	testing.LoggingSuite
	JujuOSEnvSuite
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	wrench.SetEnabled(false)
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
	s.JujuOSEnvSuite.SetUpSuite(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, false)
}

func (s *BaseSuite) TearDownSuite(c *gc.C) {
	s.JujuOSEnvSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)

	// We do this to isolate invocations of bash from pulling in the
	// ambient user environment, and potentially affecting the tests.
	// We can't always just use IsolationSuite because we still need
	// PATH and possibly a couple other envars.
	s.PatchEnvironment("BASH_ENV", "")
	network.ResetGobalPreferIPv6()
}

func (s *BaseSuite) TearDownTest(c *gc.C) {
	s.JujuOSEnvSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	s.CleanupSuite.TearDownTest(c)
}

// CheckString compares two strings. If they do not match then the spot
// where they do not match is logged.
func CheckString(c *gc.C, value, expected string) {
	if !c.Check(value, gc.Equals, expected) {
		diffStrings(c, value, expected)
	}
}

func diffStrings(c *gc.C, value, expected string) {
	// If only Go had a diff library.
	vlines := strings.Split(value, "\n")
	elines := strings.Split(expected, "\n")
	vsize := len(vlines)
	esize := len(elines)

	if vsize < 2 || esize < 2 {
		return
	}

	smaller := elines
	if vsize < esize {
		smaller = vlines
	}

	for i := range smaller {
		vline := vlines[i]
		eline := elines[i]
		if vline != eline {
			c.Logf("first mismatched line (%d/%d):", i, len(smaller))
			c.Log("expected: " + eline)
			c.Log("got:      " + vline)
			break
		}
	}
}

// TestCleanup is used to allow DumpTestLogsAfter to take any test suite
// that supports the standard cleanup function.
type TestCleanup interface {
	AddCleanup(testing.CleanupFunc)
}

// DumpTestLogsAfter will write the test logs to stdout if the timeout
// is reached.
func DumpTestLogsAfter(timeout time.Duration, c *gc.C, cleaner TestCleanup) {
	done := make(chan interface{})
	go func() {
		select {
		case <-time.After(timeout):
			fmt.Printf(c.GetTestLog())
		case <-done:
		}
	}()
	cleaner.AddCleanup(func(_ *gc.C) {
		close(done)
	})
}
