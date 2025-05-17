# Write a unit test
> See also: {ref}`unit-testing`

This document demonstrates how to write a unit test for Juju.

## Prepare for the test

### Create `package_test.go`

[note type=caution]
This step is necessary only if this file doesn't already exist.
[/note]

Each package requires a `package_test.go` file if we wish any of our tests to run.

Below is a standard `package_test.go` file for an example package called `magic`. We import the "testing" package from
the standard library and then the `tc` package. We also create a function `Test` that will be the
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

  "github.com/juju/tc"
)

func Test(t *testing.T) {
  tc.TestingT(t)
}
```

[note type=caution]
You will sometimes see `package_test.go` files which use `testing.MgoTestPackage` as their entrypoint. This is required
to run old-style `JujuConnSuite` tests, which test against a running instance of MongoDB.

These tests are deprecated and are actively being removed. No more should be added.
[/note]

### Create

`

In the code directory, for each file that you want to test (say, a source code file called `magic1.go`), create a
`<code-filename>_test.go`  (e.g. `magic1_test.go`).

## Import `tc`
> See also:  `tc` is based on [`gocheck`]( https://labix.org/gocheck)

In `magic1_test.go`, import the `tc` package:

```go
import (
"github.com/juju/tc"
)
```

### Add a unit test suite

> See also: {ref}`unit-test-suite`

Also in `magic1_test.go`, add a unit test suite.

> See more: {ref}`create-a-unit-test-suite`

Once the test suite structure has been created, it needs to be registered with `tc` or the tests will not run. You can
do by passing a pointer to an instance of our suite to the `tc.Suite` function.

```go
type magicSuite struct{}

func TestMagicSuite(t *testing.T) {
  tc.Run(t, &magicSuite{})
}
```

## Write the test

> See also: {ref}`checker`

In `magic1_test.go`, below the test suite, start adding your unit test functions.

The process is as follows: You target some behavior (usually a function) in the code file (in our case, `magic1`). You
then write a test for it, where the test usually follows the same Given, When, Then logic.

For example, suppose your `magic1.go` file defines a simple function called `Sum`:

```go
func Sum(a, b int) int {
return a + b
}
```

Then, in your `magic1_test.go` file you can write a test for it as follows (where `tc.Equals` is
a {ref}`checker <checker>`:

```go
// GIVEN a equals 5 AND b equals 3
// WHEN a and b are summed
// THEN we get 8
func (s *magicSuite) TestSum(c *tc.C) {
a := 5
b := 3

res := magic.Sum(a, b)

c.Assert(res, tc.Equals, 8)
}
```

## Run the test

Finally, to run the test, do:

```bash
go test github.com/juju/juju/x/y/magic/
```

This will run all the tests registered in the `magic` package, including the one we just wrote.

You can also chose to run specific tests or suites, using the normal go test.

## Debug a test

If you need to reproduce a failing test but can’t reproduce it easily, use this script: `juju/scripts/unit-test/stress-race.bash`.

```{tip}
**Where to run?** Running on a small or medium instance on AWS will likely help trigger races more quickly than your local hardware. Particularly useful are instances that shares CPU time -- `t._n_` instances currently. If you locally build the test to
stress you may still need to rsync over the build environment as some tests look for files in the build tree. You'll also need to install mongo.
```

```{tip}
**How many times to run?** It has been noticed that, if the test runs 100 times without failure, things are probably all right.
```
