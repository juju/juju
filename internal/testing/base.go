// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/os/v2/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/featureflag"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/internal/wrench"
	"github.com/juju/juju/juju/osenv"
)

var logger = internallogger.GetLogger("juju.testing")

// JujuOSEnvSuite isolates the tests from Juju environment variables.
// This is intended to be only used by existing suites, usually embedded in
// BaseSuite and in FakeJujuXDGDataHomeSuite. Eventually the tests relying on
// JujuOSEnvSuite will be converted to use the IsolationSuite in
// github.com/juju/testing, and this suite will be removed.
// Do not use JujuOSEnvSuite when writing new tests.
type JujuOSEnvSuite struct {
	oldHomeEnv          string
	oldEnvironment      map[string]string
	initialFeatureFlags string
}

func (s *JujuOSEnvSuite) SetUpTest(c *gc.C) {
	s.oldEnvironment = make(map[string]string)
	for _, name := range []string{
		osenv.JujuXDGDataHomeEnvKey,
		osenv.JujuControllerEnvKey,
		osenv.JujuModelEnvKey,
		osenv.JujuLoggingConfigEnvKey,
		osenv.JujuFeatureFlagEnvKey,
		osenv.JujuFeatures,
		osenv.XDGDataHome,
	} {
		s.oldEnvironment[name] = os.Getenv(name)
		os.Setenv(name, "")
	}
	s.oldHomeEnv = utils.Home()
	os.Setenv(osenv.JujuXDGDataHomeEnvKey, c.MkDir())
	err := utils.SetHome("")
	c.Assert(err, jc.ErrorIsNil)

	// Update the feature flag set to be the requested initial set.
	// For tests, setting with the environment variable isolates us
	// from a single resource that was hitting contention during parallel
	// test runs.
	os.Setenv(osenv.JujuFeatureFlagEnvKey, s.initialFeatureFlags)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *JujuOSEnvSuite) TearDownTest(c *gc.C) {
	for name, value := range s.oldEnvironment {
		os.Setenv(name, value)
	}
	err := utils.SetHome(s.oldHomeEnv)
	c.Assert(err, jc.ErrorIsNil)
}

// SkipIfPPC64EL skips the test if the arch is PPC64EL and the
// compiler is gccgo.
func SkipIfPPC64EL(c *gc.C, bugID string) {
	if runtime.Compiler == "gccgo" &&
		arch.NormaliseArch(runtime.GOARCH) == arch.PPC64EL {
		c.Skip(fmt.Sprintf("Test disabled on PPC64EL until fixed - see bug %s", bugID))
	}
}

// SkipIfS390X skips the test if the arch is S390X.
func SkipIfS390X(c *gc.C, bugID string) {
	if arch.NormaliseArch(runtime.GOARCH) == arch.S390X {
		c.Skip(fmt.Sprintf("Test disabled on S390X until fixed - see bug %s", bugID))
	}
}

// SkipIfWindowsBug skips the test if the OS is Windows.
func SkipIfWindowsBug(c *gc.C, bugID string) {
	if runtime.GOOS == "windows" {
		c.Skip(fmt.Sprintf("Test disabled on Windows until fixed - see bug %s", bugID))
	}
}

// SkipUnlessControllerOS skips the test if the current OS is not a supported
// controller OS.
func SkipUnlessControllerOS(c *gc.C) {
	if coreos.HostOS() != ostype.Ubuntu {
		c.Skip("Test disabled for non-controller OS")
	}
}

// SkipLXDNotSupported will skip tests if the host does not support LXD
func SkipLXDNotSupported(c *gc.C) {
	if coreos.HostOS() != ostype.Ubuntu {
		c.Skip("Test disabled for non-LXD OS")
	}
}

// SkipFlaky skips the test if there is an open bug for intermittent test failures
func SkipFlaky(c *gc.C, bugID string) {
	c.Skip(fmt.Sprintf("Test disabled until flakiness is fixed - see bug %s", bugID))
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
	logger.Debugf(context.TODO(), "setting feature flags: %s", flags)
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
	oldLtsForTesting string
	testing.CleanupSuite
	testing.LoggingSuite
	JujuOSEnvSuite
	InitialLoggingConfig string
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	wrench.SetEnabled(false)
	s.CleanupSuite.SetUpSuite(c)
	s.LoggingSuite.SetUpSuite(c)
	// JujuOSEnvSuite does not have a suite setup.
	// LTS-dependent requires new entry upon new LTS release.
	s.oldLtsForTesting = series.SetLatestLtsForTesting("xenial")
}

func (s *BaseSuite) TearDownSuite(c *gc.C) {
	// JujuOSEnvSuite does not have a suite teardown.
	_ = series.SetLatestLtsForTesting(s.oldLtsForTesting)
	s.LoggingSuite.TearDownSuite(c)
	s.CleanupSuite.TearDownSuite(c)
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	if s.InitialLoggingConfig != "" {
		_ = internallogger.ConfigureLoggers(s.InitialLoggingConfig)
	}

	// We do this to isolate invocations of bash from pulling in the
	// ambient user environment, and potentially affecting the tests.
	// We can't always just use IsolationSuite because we still need
	// PATH and possibly a couple other envars.
	s.PatchEnvironment("BASH_ENV", "")
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
	AddCleanup(func(*gc.C))
}

// DumpTestLogsAfter will write the test logs to stdout if the timeout
// is reached.
func DumpTestLogsAfter(timeout time.Duration, c *gc.C, cleaner TestCleanup) {
	done := make(chan interface{})
	go func() {
		select {
		case <-time.After(timeout):
			fmt.Print(c.GetTestLog())
		case <-done:
		}
	}()
	cleaner.AddCleanup(func(_ *gc.C) {
		close(done)
	})
}

// GetExportedFields return the exported fields of a struct.
func GetExportedFields(arg interface{}) set.Strings {
	t := reflect.TypeOf(arg)
	result := set.NewStrings()

	count := t.NumField()
	for i := 0; i < count; i++ {
		f := t.Field(i)
		// empty PkgPath means exported field.
		// see https://golang.org/pkg/reflect/#StructField
		if f.PkgPath == "" {
			result.Add(f.Name)
		}
	}

	return result
}

// CurrentVersion returns the current Juju version, asserting on error.
func CurrentVersion() version.Binary {
	return version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
}

// HostSeries returns series.HostSeries(), asserting on error.
func HostBase(c *gc.C) base.Base {
	hostBase, err := coreos.HostBase()
	c.Assert(err, jc.ErrorIsNil)
	return hostBase
}
