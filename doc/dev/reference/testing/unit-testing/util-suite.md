<!--TODO:
- Include more base test suites. E.g., LoggingCleanupSuite, OsEnvSuite:
https://github.com/juju/testing
- 
-->
> *[Testing](../index.md) > [Unit testing](index.md) > Util suite*


A **util suite** is a struct which provides specific setup and teardown behavior as well as useful variables and
functions to write [unit test](unit-test-suite.md) suites with.

There are several util suites, among which:

* `/testing.BaseSuite` (`testing/base.go`)
* `/testing.FakeHomeSuite` (`testing/environ.go`)
* `/testing.FakeJujuHomeSuite` (`testing/environ.go`)
* (DEPRECATED) `/juju/testing.JujuConnSuite` (`juju/testing/conn.go`)

The last three have the BaseSuite functionality included through composition. The BaseSuite isolates a user's home
directory from accidental modification (by setting `$HOME to ""`) and errors if there is an attempt to do outgoing HTTP
access. It also clears the relevant `$JUJU_* environment variables`. The `BaseSuite` is also composed of the core
`LoggingSuite`, and also `LoggingSuite` from `github.com/juju/testing`, which brings in the `CleanupSuite` from the
same. The `CleanupSuite` has functionality around patching environment variables and normal variables for the duration
of a test. It also provides a clean-up stack called when the test teardown happens.

If you need additional functionality, you can embed one of the suites below:

* BaseSuite

* FakeHomeSuite: creates a fake home directory with `~/.ssh` and fake ssh keys.
* FakeJujuHomeSuite: as above but also sets up a `~/.config/juju` with a fake model.

* (DEPRECATED) JujuConnSuite: Simulates a controller and an API server as well as a database. Quite heavy weight. Can be
  useful in that it simulates a Juju environment as closely as possible.