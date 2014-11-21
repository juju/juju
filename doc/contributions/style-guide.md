# Style Guide

This document is a guide to aid juju-core reviewers.

## Contents

1. [Pull Requests](#pull-requests)
2. [Comments](#comments)
3. [Functions](#functions)
4. [Errors](#errors)
5. [Tests](#tests)
6. [API](#api)


## Pull Requests

A Pull Request (PR) needs a justification. This could be: 

- An issue, ie “Fixes LP 12349000”
- A description justifying the change
- A link to a design document or mailing list discussion

PRs should be kept as small as possible, addressing one discrete issue outlined 
in the PR justification. The smaller the diff, the better. 

If your PR addresses several distinct issues, please split the proposal into one 
PR per issue. 

Once your PR is up and reviewed, don't comment on the review saying "i've done this", 
unless you've pushed the code.

## Comments

Comments should be full sentences with one space after a period, proper 
capitalization and punctuation.

Avoid adding non-useful information, such as flagging the layout of a file with 
block comments or explaining behaviour that is immediately evident from reading 
the code.

All exported symbols (names, functions, methods, types, constants and variables) 
must have a comment.

Anything that is unintuitive at first sight must have a comment, such as:

- non-trivial unexported type or function declarations (e.g. if a type implements an interface)
- the breaking of a convention (e.g. not handling an error)
- a particular behaviour the reader would have to think 'more deeply' about to understand

Top-level name comments start with the name of the thing. For example, a top-level, 
exported function:

```go
// AwesomeFunc does awesome stuff.
func AwesomeFunc(i int) (string, error) {
        // ...
        return someString, err
}
```

A TODO comment needs to be linked to a bug in Launchpad. Include the lp bug number in 
the TODO with the following format:

```go
// TODO(username) yyyy-mm-dd bug #1234567 
```

e.g.

```go
// TODO(axw) 2013-09-24 bug #1229507
```

Each file must have a copyright notice. The copyright notice must include the year 
the file was created and the year the file was last updated, if applicable.

```go
// Copyright 2013-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main
```

Note that the blank line following the notice separates the comment from the package, 
ensuring it does not appear in godoc.

## Functions

What the function does should be evident by its signature. 

How a function does what it does should also be clear. If a function is large 
and verbose, breaking apart it's business logic into helper functions can make 
it easier to read. Conversely, if the function's logic is too opaque, 
in-lining a helper function or variable may help.

If a function has named return values in it's signature, it should use a 
bare return:

```go
func AwesomeFunc(i int) (s string, err error) {
        // ...
        return // Don't use the statement: "return s, err"
}
```

## Errors

The text of error messages should be lower case without a trailing period, 
because they are often included in other strings or output:

```go
// INCORRECT
return fmt.Errorf("Cannot read  config %q.", configFilePath)

// CORRECT
return fmt.Errorf("cannot read agent config %q", configFilePath)
```

If a function call only returns an error, assign and handle the error 
within one if statement (unless doing so makes the code harder to read, 
for example if the line of the function call is already quite long):

```go
// PREFERRED
if err := someFunc(); err != nil {
    return err
}
```

If an error is thrown away, why it is not handled is explained in a comment.

The juju/errors package should be used to handle errors:

```go
// Trace always returns an annotated error.  Trace records the
// location of the Trace call, and adds it to the annotation stack.
// If the error argument is nil, the result of Trace is nil.
if err := SomeFunc(); err != nil {
      return errors.Trace(err)
}

// Errorf creates a new annotated error and records the location that the
// error is created.  This should be a drop in replacement for fmt.Errorf.
return errors.Errorf("validation failed: %s", message)

// Annotate is used to add extra context to an existing error. The location of
// the Annotate call is recorded with the annotations. The file, line and
// function are also recorded.
// If the error argument is nil, the result of Annotate is nil.
if err := SomeFunc(); err != nil {
    return errors.Annotate(err, "failed to frombulate")
}
```

See [github.com/juju/errors/annotation.go](github.com/juju/errors/annotation.go) for more error handling functions.

## Tests

- Please read ../doc/how-to-write-tests.txt
- Each test should test a discrete behaviour and is idempotent
- The behaviour being tested is clear from the function name, as well as the 
  expected result (i.e. success or failure)
- Repeated boilerplate in test functions is extracted into it's own 
  helper function or `SetUpTest`
- External functionality, that is not being directly tested, should be mocked out

Variables in other modules are not patched directly. Instead, create a local 
variable and patch that:

```go
// ----------
// in main.go
// ----------

import somepackage

var someVar = somepackage.SomeVar

// ---------------
// in main_test.go
// ---------------

func (s *SomeSuite) TestSomethingc(c *gc.C) {
        s.PatchValue(someVar, newValue)
        // ....
}
```

If your test functions are under `package_test` and they need to test something 
from package that is not exported, create an exported alias to it in `export_test.go`:

```go
// -----------
// in tools.go
// -----------

package tools

var someVar = value

// -----------------
// in export_test.go
// -----------------

package tools

var SomeVar = someVar

// ---------------
// in main_test.go
// ---------------

package tools_test

import tools

func (s *SomeSuite) TestSomethingc(c *gc.C) {
        s.PatchValue(tools.SomeVar, newValue)
        // ...
}
```

## Layout

Imports are grouped into 3 sections: standard library, 3rd party libraries, juju/juju library:

```go
import (
    "fmt"
    "io"

    "github.com/juju/errors"
    "github.com/juju/loggo"
    gc "gopkg.in/check.v1"

    "github.com/juju/juju/environs"
    "github.com/juju/juju/environs/config"
)
```

## API

- Please read ../doc/api.txt
- Client calls to the API are, by default, batch calls
