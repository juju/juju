(styleguide)=
# Styleguide

This document is a guide to aid juju-core reviewers.

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

## CLI commands

The base `Command` interface is found in `cmd/cmd.go`.

Commands need to provide an `Info` method that returns an Info struct.

The info struct contains: name, args, purpose and a detailed description.
This information is used to provide the default help for the command.

In the same package, there is `CommandBase` whose purpose is to be composed
into new commands, and provides a default no-op SetFlags implementation, a
default Init method that checks for no extra args, and a default Help method.


### Supercommands

`Supercommand`s are commands that do many things, and have "sub-commands" that
provide this functionality.  Git and Bazaar are common examples of
"supercommands".  Subcommands must also provide the `Command` interface, and
are registered using the `Register` method.  The name and aliases are
registered with the supercommand.  If there is a duplicate name registered,
the whole thing panics.

Supercommands need to be created with the `NewSuperCommand` function in order
to provide a fully constructed object.

### The 'help' subcommand

All supercommand instances get a help command.  This provides the basic help
functionality to get all the registered commands, with the addition of also
being able to provide non-command help topics which can be added.

Help topics have a `name` which is what is matched from the command line, a
`short` one line description that is shown when `<cmd> help` is called,
and a `long` text that is output when the topic is requested.


### Execution

The `Main` method in the cmd package handles the execution of a command.

A new `gnuflag.FlagSet` is created and passed to the command in `SetFlags`.
This is for the command to register the flags that it knows how to handle.

The args are then parsed, and passed through to the `Init` method for the
command to decide what to do with the positional arguments.

The command is then `Run` and passed in an execution `Context` that defines
the standard input and output streams, and has the current working directory.

### Interactive commands

Some commands in Juju are interactive. To keep the UX consistent across the product, please follow these guidelines.
Note, many of these guidelines are supported by the interact package at github.com/juju/juju/cmd/juju/interact.

* All interactive commands should begin with a short blurb telling the user that they've started an interactive wizard.
  `Starting interactive bootstrap process.`
* Prompts should be a short imperative sentence with the first letter capitalized, ending with a colon and a space
  before waiting for user input.
  `Enter your mother's maiden name: `
* Prompts should tell the user what to do, not ask them questions.
  `Enter a name: ` rather than `What name do you want?`
* The only time a prompt should end with a question mark is for yes/no questions.
  `Use foobar as network? (Y/n): `
* Yes/no questions should always end with (y/n), with the default answer capitalized.
* Try to always format the question so you can make yes the default.
* Prompts that request the user choose from a list of options should start `Select a ...`
* Prompts that request the user enter text not from a list should start `Enter ....`
* Most prompts should have a reasonable default, shown in brackets just before the colon, which can be accepted by just
  hitting enter.
  `Select a cloud [localhost]: `
* Questions that have a list of answers should print out those options before the prompt.
* Options should be consistently sorted in a logical manner (usually alphabetical).
* Options should be a single short word, if possible.
* Selecting from a list of options should always be case insensitive.
* If an incorrect selection is entered, print a short error and reprint the prompt, but do not reprint the list of
  options. No blank line should be printed between the error and the corresponding prompt.
* Always print a blank line between prompts.
