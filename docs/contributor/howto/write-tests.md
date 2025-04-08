(write-tests)=
# Write tests

On the whole, new or updated code will not pass review unless there are tests
associated with the code.  For code additions, the tests should cover as much
of the new code as practical, and for code changes, either the tests should be
updated, or at least the tests that already exist that cover the refactored
code should be identified when requesting a review to show that there is already
test coverage, and that the refactoring didn't break anything.


## go test and gocheck

The `go test` command is used to run the tests.  Juju uses the `gocheck` package
("gopkg.in/check.v1") to provide a checkers and assert methods for the test
writers.  The use of gocheck replaces the standard `testing` library.

Across all of the tests in juju-core, the gocheck package is imported
with a shorter alias, because it is used a lot.

```go
import (
	// system packages

	gc "gopkg.in/check.v1"

	// juju packages
)
```


## Setting up tests for new packages

Lets say we are creating a new provider for "magic" cloud, and we have a package
called "magic" that lives at "github.com/juju/juju/provider/magic".  The
general approach for testing in juju is to have the tests in a separate package.
Continuing with this example the tests would be in a package called "magic_test".

A common idiom that has occurred in juju is to setup to gocheck hooks in a special
file called `package_test.go` that would look like this:


```go
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package magic_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}
```

or

```go
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package magic_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
```

The key difference here is that the first one just hooks up `gocheck`
so it looks for the `gocheck` suites in the package.  The second makes
sure that there is a mongo available for the duration of the package tests.

A general rule is not to setup mongo for a package unless you really
need to as it is extra overhead.


## Writing the test files

Normally there will be a test file for each file with code in the package.
For a file called `config.go` there should be a test file called `config_test.go`.

The package should in most cases be the same as the normal files with a "_test" suffix.
In this way, the tests are testing the same interface as any normal user of the
package.  It is reasonably common to want to modify some internal aspect of the package
under test for the tests.  This is normally handled by a file called `export_test.go`.
Even though the file ends with `_test.go`, the package definition is the same as the
normal source files. In this way, for the tests and only the tests, additional
public symbols can be defined for the package and used in the tests.

Here is an annotated extract from `provider/local/export_test.go`

```go
// The package is the "local" so it has access to the package symbols
// and not just the public ones.
package local

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
)

var (
	// checkIfRoot is a variable of type `func() bool`, so CheckIfRoot is
	// a pointer to that variable so we can patch it in the tests.
	CheckIfRoot      = &checkIfRoot
	// providerInstance is a pointer to an instance of a private structure.
	// Provider points to the same instance, so public methods on that instance
	// are available in the tests.
	Provider         = providerInstance
)

// ConfigNamespace is a helper function for the test that steps through a
// number of private methods or variables, and is an alternative mechanism
// to provide functionality for the tests.
func ConfigNamespace(cfg *config.Config) string {
	env, _ := providerInstance.Open(cfg)
	return env.(*localEnviron).config.namespace()
}
```

## Suites and Juju base suites

With gocheck tests are grouped into Suites. Each suite has distinct
set-up and tear-down logic.  Suites are often composed of other suites
that provide specific set-up and tear-down behaviour.

There are four main suites:

  * /testing.BaseSuite (testing/base.go)
  * /testing.FakeHomeSuite (testing/environ.go)
  * /testing.FakeJujuHomeSuite (testing/environ.go)
  * /juju/testing.JujuConnSuite (juju/testing/conn.go)

The last three have the BaseSuite functionality included through
composition.  The BaseSuite isolates a user's home directory from accidental
modification (by setting $HOME to "") and errors if there is an attempt to do
outgoing http access. It also clears the relevant $JUJU_* environment variables.
The BaseSuite is also composed of the core LoggingSuite, and also LoggingSuite
from  github.com/juju/testing, which brings in the CleanupSuite from the same.
The CleanupSuite has the functionality around patching environment variables
and normal variables for the duration of a test. It also provides a clean-up
stack that gets called when the test teardown happens.

All test suites should embedd BaseSuite. Those that need the extra functionality
can instead embedd one of the fake home suites:

* FakeHomeSuite: creates a fake home directory with ~/.ssh and fake ssh keys.
* FakeJujuHomeSuite: as above but also sets up a ~/.config/juju with a fake model.

The JujuConnSuite does this and more. It also sets up a controller and api
server.  This is one problem with the JujuConnSuite, it almost always does a
lot more than you actually want or need.  This should really be broken into
smaller component parts that make more sense.  If you can get away with not
using the JujuConnSuite, you should try.

To create a new suite composed of one or more of the suites above, you can do
something like:

```go
type ToolsSuite struct {
	testing.BaseSuite
	dataDir string
}

var _ = gc.Suite(&ToolsSuite{})

```

If there is no extra setup needed, then you don't need to specify any
set-up or tear-down methods as the LoggingSuite has them, and they are
called by default.

If you did want to do something, say, create a directory and save it in
the dataDir, you would do something like this:

```go
func (t *ToolsSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)
	t.dataDir = c.MkDir()
}
```

If the test suite has multiple contained suites, please call them in the
order that they are defined, and make sure something that is composed from
the BaseSuite is first.  They should be torn down in the reverse order.

Even if the code that is being tested currently has no logging or outbound
network access in it, it is a good idea to use the BaseSuite as a base:
 * it isolates the user's home directory against accidental modification
 * if someone does add outbound network access later, it will be caught
 * it brings in something composed of the CleanupSuite
 * if someone does add logging later, it is captured and doesn't pollute
   the logging output


## Patching variables and the environment

Inside a test, and assuming that the Suite has a CleanupSuite somewhere
in the composition tree, there are a few very helpful functions.

```go

var foo int

func (s *someTest) TestFubar(c *gc.C) {
	// The TEST_OMG environment value will have "new value" for the duration
	// of the test.
	s.PatchEnvironment("TEST_OMG", "new value")

	// foo is set to the value 42 for the duration of the test
	s.PatchValue(&foo, 42)
}
```

PatchValue works with any matching type. This includes function variables.


## Checkers

Checkers are a core concept of `gocheck` and will feel familiar to anyone
who has used the python testtools.  Assertions are made on the gocheck.C
methods.

```go
c.Check(err, jc.ErrorIsNil)
c.Assert(something, gc.Equals, somethingElse)
```

The `Check` method will cause the test to fail if the checker returns
false, but it will continue immediately cause the test to fail and will
continue with the test. `Assert` if it fails will cause the test to
immediately stop.

For the purpose of further discussion, we have the following parts:

	`c.Assert(observed, checker, args...)`

The key checkers in the `gocheck` module that juju uses most frequently are:

	* `IsNil` - the observed value must be `nil`
	* `NotNil` - the observed value must not be `nil`
	* `Equals` - the observed value must be the same type and value as the arg,
	  which is the expected value
	* `DeepEquals` - checks for equality for more complex types like slices,
	  maps, or structures. This is DEPRECATED in favour of the DeepEquals from
	  the `github.com/juju/testing/checkers` covered below
	* `ErrorMatches` - the observed value is expected to be an `error`, and
	  the arg is a string that is a regular expression, and used to match the
	  error string
	* `Matches` - a regular expression match where the observed value is a string
    * `HasLen` - the expected value is an integer, and works happily on nil
      slices or maps


Over time in the juju project there were repeated patterns of testing that
were then encoded into new and more complicated checkers.  These are found
in `github.com/juju/testing/checkers`, and are normally imported with the
alias `jc`.

The matchers there include (not an exclusive list):

	* `IsTrue` - just an easier way to say `gc.Equals, true`
	* `IsFalse` - observed value must be false
	* `GreaterThan` - for integer or float types
	* `LessThan` - for integer or float types
	* `HasPrefix` - obtained is expected to be a string or a `Stringer`, and
	  the string (or string value) must have the arg as start of the string
	* `HasSuffix` - the same as `HasPrefix` but checks the end of the string
	* `Contains` - obtained is a string or `Stringer` and expected needs to be
	  a string. The checker passes if the expected string is a substring of the
	  obtained value.
	* `DeepEquals` - works the same way as the `gocheck.DeepEquals` except
	  gives better errors when the values do not match
	* `SameContents` - obtained and expected are slices of the same type,
	  the checker makes sure that the values in one are in the other. They do
	  not have the be in the same order.
	* `Satisfies` - the arg is expected to be `func(observed) bool`
	  often used for error type checks
	* `IsNonEmptyFile` - obtained is a string or `Stringer` and refers to a
	  path. The checker passes if the file exists, is a file, and is not empty
	* `IsDirectory` - works in a similar way to `IsNonEmptyFile` but passes if
	  the path element is a directory
	* `DoesNotExist` - also works with a string or `Stringer`, and passes if
	  the path element does not exist



## Good tests

Good tests should be:
  * small and obviously correct
  * isolated from any system or model values that may impact the test
