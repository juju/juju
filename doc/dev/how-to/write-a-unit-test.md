> See also: [Unit testing](/doc/dev/reference/testing/unit-testing/index.md)

This document demonstrates how to write a unit test for Juju.

<!-- TOC -->

* [Prepare for the test](#prepare-for-the-test)
  * [Create `package_test.go`](#create-package_testgo)
  * [Create](#create)
  * [Import `gocheck`](#import-gocheck)
  * [Add a unit test suite](#add-a-unit-test-suite)
* [Write the test](#write-the-test)
* [Run the test](#run-the-test)

<!-- TOC -->

# Prepare for the test

## Create `package_test.go`

[note type=caution]
This step is necessary only if this file doesn't already exist.
[/note]

Each package requires a `package_test.go` file if we wish any of our tests to run.

Below is a standard `package_test.go` file for an example package called `magic`. We import the "testing" package from
the standard library and then the `gocheck` package as `gc`. We also create a function `Test` that will be the
entry-point into our test suites.

<!--?loads the test suites that have been added to a list by var in the "HTG create a test suite"-->
<!-- // TestingT runs all test suites registered with the Suite function,
// printing results to stdout, and reporting any failures back to
// the "testing" package.-->

```go
// Copyright 20XX Canonical Ltd.
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

[note type=caution]
You will sometimes see `package_test.go` files which use `testing.MgoTestPackage` as their entrypoint. This is required
to run old-style `JujuConnSuite` tests, which test against a running instance of MongoDB.

These tests are deprecated and are actively being removed. No more should be added.
[/note]

## Create

`

In the code directory, for each file that you want to test (say, a source code file called `magic1.go`), create a
`<code-filename>_test.go`  (e.g. `magic1_test.go`).

## Import `gocheck`
> See also:  [`gocheck`]( https://labix.org/gocheck)

In `magic1_test.go`, import the `gocheck` package as `gc`:

```go
import (
gc "gopkg.in/check.v1"
)
```

[note type=information]
`gc` is the usual alias for [gocheck](https://labix.org/gocheck) across all the Juju repositories.
[/note]

## Add a unit test suite

> See also: [Unit test suite](/doc/dev/reference/testing/unit-testing/unit-test-suite.md)

Also in `magic1_test.go`, add a unit test suite.

> See more: [How to create a unit test suite](create-unit-test-suite.md)

Once the test suite structure has been created, it needs to be registered with `gc` or the tests will not run. You can
do by passing a pointer to an instance of our suite to the `gc.Suite` function.

```go
type magicSuite struct{}

var _ = gc.Suite(&magicSuite{})
```

# Write the test

> See also: [Checker](/doc/dev/reference/testing/unit-testing/checker.md)

In `magic1_test.go`, below the test suite, start adding your unit test functions.

The process is as follows: You target some behavior (usually a function) in the code file (in our case, `magic1`). You
then write a test for it, where the test usually follows the same Given, When, Then logic.

For example, suppose your `magic1.go` file defines a simple function called `Sum`:

```go
func Sum(a, b int) int {
return a + b
}
```

Then, in your `magic1_test.go` file you can write a test for it as follows (where `gc.Equals` is
a [Checker](/doc/dev/reference/testing/unit-testing/checker.md)):

```go
// GIVEN a equals 5 AND b equals 3
// WHEN a and b are summed 
// THEN we get 8
func (s *magicSuite) TestSum(c *gc.C) {
a := 5
b := 3

res := magic.Sum(a, b)

c.Assert(res, gc.Equals, 8)
}
```

# Run the test

Finally, to run the test, do:

```bash
go test github.com/juju/juju/x/y/magic/
```

This will run all the tests registered in the `magic` package, including the one we just wrote.

You can also chose to run specific tests or suites, using the `-check.f` flag for gocheck

```bash
go test github.com/juju/juju/x/y/magic/ -check.f magicSuite         # run the magicSuite only
go test github.com/juju/juju/x/y/magic/ -check.f magicSuite.TestSum # run the test TestSum in magicSuite only
```

[note type=information]
See more here [`gocheck` > Selecting which tests to run](https://labix.org/gocheck) .
[/note]


> <small>Contributors: @jack-shaw  </small>