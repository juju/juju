---
myst:
  html_meta:
    description: "Learn about unit testing in Juju with gocheck package, including test suites, checkers, and util suite composition patterns."
---

(unit-testing)=
# Unit testing

```{toctree}
:titlesonly:
:glob:

*
```

> See also: {ref}`write-a-unit-test`

Unit testing `juju` currently involves the [`gocheck` package](https://labix.org/gocheck) and the `go test` CLI
command.

A typical unit testing package is placed alongside the code package that you want to test (in the same directory) and
consists of:

- A file `package_test.go`, which is the entrypoint to your unit testing.

- (OPTIONAL) A file `export_test.go`, which is used to access or modify some internal aspect of the package under test
  for the tests. Even though the file ends with `_test.go`, the package definition is the same as the normal source
  files. In this way, for the tests and only the tests, additional variables and functions can be defined for the
  package and used in the tests.

- Test files of the form `<code filename>_test.go`, which are collections of {ref}`unit test suites <unit-test-suite>`.

