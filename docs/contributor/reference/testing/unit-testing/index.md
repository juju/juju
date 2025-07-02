(unit-testing)=
# Unit testing

```{toctree}
:titlesonly:
:glob:

*
```

> See also: [How to write a unit test](/doc/dev/how-to/write-a-unit-test.md)

Unit testing `juju` uses the `go test` CLI and a test suite and check libary called [`tc`](https://github.com/juju/tc").

A typical unit testing package is placed alongside the code package that you want to test (in the same directory) and
consists of:

- A file `package_test.go`, which is the entrypoint to your unit testing.

- (OPTIONAL) A file `export_test.go`, which is used to access or modify some internal aspect of the package under test
  for the tests. Even though the file ends with `_test.go`, the package definition is the same as the normal source
  files. In this way, for the tests and only the tests, additional variables and functions can be defined for the
  package and used in the tests.

<!--Even though it's part of the `magic` package rather than the `test` package, we call it `_test` because Juju ignores all files with `_test`. This file acts like a bridge between the two packages.
-->

- Test files of the form `<code filename>_test.go`, which are collections of unit test suites.

<!--
github.com/juju/juju/provider/magic:

- magic1.go

- magic1_test.go

- magic2.go

- magic2_test.go

- magic3.go

- magic3_test.go

-->
<!-- to provide a checkers and assert methods for the test writers.**-->

> See more:
> - [Unit test suite](unit-test-suite.md)
    >     - [Util suite](util-suite)
> - [Checker](checker.md)
