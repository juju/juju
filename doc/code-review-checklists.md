# Contents

* [Introduction](#introduction)
* [Using the checklists](#using-the-checklists)
* Checklists
    + [General](#general)
    + [state](#state)
    + [apiserver](#apiserver)
    + [api](#api)
    + [Workers](#workers)
    + [CLI and hook tools](#cli--hook-tools)

# Introduction

This document contains checklists to use when reviewing changes to
Juju core. They have been synthesised from various documents written
over Juju’s history as well as issues that have been raised on actual
reviews. These checklists don't cover every possible thing that a
reviewer should be thinking of when looking at a proposed change. The
focus is on the biggest wins that can be practically included in a
checklist.

The hope is that the checklists will help to ensure a base level of
quality and assist reviewers to be in the right frame of mind when
evaluating pull requests.

These checklists are not set in stone. They will evolve as Juju and
the team's development practices evolve. Any significant changes to
the checklists should be proposed via the juju-dev mailing list of the
Juju tech board.

## Using the checklists

- For each pull request at least one reviewer must go through the
  checklists relevant to the pull request.
- The reviewer should indicate that they have gone through the
  checklist(s). Any issues found via the checklist(s) should be raised
  on the review.
- The "General" list applies to **all** pull requests.
- There a checklists which apply to specific areas of the code which
  should be applied as applicable.

As a reviewer, a reasonable approach seems to be to read through a
proposed change once, and then apply the checklists once you have it
in your head. YMMV.

# General

- Does the pull request description justify the change?
- If the pull request is a bug fix, does the description link to the relevant ticket?
- Have QA steps been defined?
    * Or justification why external QA is not possible
- Is all state non-global?
    * There should be no global variables/state.
    * _package level logging is an excepted_
- Are there unit tests with reasonable coverage?
- Are the unit tests actually unit tests?
    * Wherever possible, unit tests should not involve functionality
      outside of the unit being tested.
    * JujuConnSuite should not be used for new unit tests
- Do the test exercise behaviour, not implementation?
    * Correct behaviour is far more important than how it is done
- Are tests isolated from the host machine’s environment?
    * Start with IsolationSuite as a base but know that it only
      addresses some isolation concerns.
- Are all test suites registered with gocheck? (`gc.Suite(...)`)
- Do tests prefer AddCleanup over TearDownTest or TearDownSuite?
- Are external dependencies passed in?
    * Use interfaces!
    * Patching in tests should be kept to a minimum.
- Do structs, methods and funcs each have just one, clear job?
    * Sprawling, complex units of code are bug prone and hard to reason about
- Do all exported structs, fields, methods and funcs have docstrings?
- Do new packages have documentation? (in doc.go)
    * These should explain why the package exists with references to the important parts.
- Has the documentation for preexisting packages been updated to match the changes made?
- Are non-obvious parts of the implementation explained with comments?
- Are non-obvious parts of the implementation as clear as they could be?
    * Sometimes, restructuring the code, or choosing better names for
      things, can make the code much clearer.
- Are inputs validated?
    * Don't trust anyone or anything.
- Have sensible names been used?
    * Say what something does, not how it works.
    * Don't use names which may confuse or mislead.
- Are channels used in favour of sharing memory wherever possible?
    * Primitives from the sync package should be used sparingly
    * Sync package primitives such as Mutex are ok to protect state
      internal to a type but should never be exposed externally.
- Are all created resources, cleaned up later?
    * There must be a something in place to clean up each API
      connection, mongo session, goroutine and worker.
    * If you start it, you must stop it.
- Are all channel reads and writes abortable?
    * All channel reads and writes must be performed using a select
      which also involves some other channel which can be used to abort
      the read or write.
    * The only exception to this is in tests where it can be useful to
      write into a buffered channel.
- Are channels private wherever possible?
    * `<-chan struct{}` channels which just get closed are probably fine
- Are interfaces as small as they can be?
    * Consider whether larger interfaces could be split up.
- For code that deals with filenames, OS specific errors or low level operations, will it work on all platforms we care
  about?
    * Paths work differently on different platofrms
    * Errors should not be examined for specific text as these can vary by platform.
- Are errors being created and wrapped using the juju/errors package?
- Is utils.Clock used instead of time.Now or time.After?
- Do all files contain an copyright header?
- Is there a useful and tasteful amount of logging?
    * Not too little; not too much
- Have appropriate log levels been used for each logged message?
    * INFO and up for the user
    * DEBUG and down for developers
    * Use the right level within those ranges

# state

- State code does not depend in any way on higher layers, especially on apiserver and api?
- Do state watchers always send an initial event?
- Does domain data and logic live outside of state wherever possible?
    * Under `core/` or in a top level package.
- Are all updates to the database made using transactions?
- Is model specific data stored in model filtering collections?
    * Check the collection definition in allcollections.go
- Are uses of getRawCollection actually necessary?
    * getCollection should be used in almost most cases
- Are transaction assertion failures, especially database write races, adequately covered by the implementation?
    * There should be a database query and check for each transaction assert
- Are database write races covered by the tests?
    * Using the SetBeforeHooks and SetAfterHooks helpers
- Are time values stored to the DB using int64 instead of time.Time?
    * time.Time only has millisecond precision
- Are entity global keys stored to the database instead of tags?
    * Tags are for the API, global keys are for the database
- Are database document structs, database global keys and database related errors (e.g. ErrAborted) kept inside the
  state package?
- Are collections and sessions always closed after use?
- Are the collection name constants (e.g. machinesC) always used instead of string literals?

# apiserver

Example: https://github.com/juju/juju/tree/master/apiserver/sshclient

- apiserver code only depends on state and core/*, not api layer?
- Are API facades, methods and packages documented to level suitable for 3rd parties?
- Do facades and methods have the required authorization checks?
    * APIs should not be overly permissive.
- Is ErrPerm returned for cases of insufficient authorization and unknown entities?
- Is an error other than ErrPem used when input is invalid or missing?
    * For usability reasons, mistakes by the client shouldn't be obscured with ErrPerm
- Are identifiers for machines, units, users, models etc always sent as stringified tags?
- Have new tag types been defined and used for new Juju concepts?
    * If a new Juju concept has an identifier which is sent over the
      API, then a new Juju tag type should be defined for it.
- Do all struct field names exposed over the API have json struct tags?
- Do all struct field json field names use “lower-case-with-dashes” format?
- Do fields which hold a tag have a name which ends with "-tag"?
- Do all API methods support bulk operation?
- Have new API facade versions been created when there’s an incompatible change?
- When a new API facade version has been added, has the previous version - including tests - been preserved?
- Is functionality that could be shared between facades pulled out to apiserver/common?
- Are facades as thin and simple as possible with as much logic as possible living in lower layers or under
  apiserver/common?
- Is each facade focussed on a single role or type of client?
    * Facade should group methods for things that will change together.

# api

Example: https://github.com/juju/juju/tree/master/api/migrationflag

- api code only depends on apiserver/params and not other apiserver packages?
- Do client side api calls return business logic structs and constants
  from core/* or other top level packages?
    * structs from apiserver/params shouldn't be returned
- Are there checks to ensure the correct number of results are returned from bulk apiserver methods?
- Do the tests use a mock of the apiserver facade instead of hitting
  the actual API?
- Are client side API facades thin with business logic pushed out to
  workers and top level packages?
- Are there signature changes - changes in calls input/output/rename - that require version bumping?

# Workers

Example: https://github.com/juju/juju/tree/master/worker/hostkeyreporter

- Does each worker only depend on client side “api” layer for
  interaction with controller?
    * not apiserver and certainly not state
- Does each worker use a tomb.Tomb or catacomb.Catacomb?
- For workers which use watchers or have internal long running
  goroutines, has catacomb.Catacomb been used?
    * Catacomb takes care of ensuring that all registered goroutines die together
- Are all long running operations - including channel operations -
  interruptible by the Tomb/Catacomb’s Dying channel?
- Is ErrDying prevented from leaking out of the worker?
- Are all resources created by the worker cleaned up before the
  tomb/catacomb is marked as dead (i.e when tomb.Done is called)?
    * Hint: Catacomb makes this easier to get right
- Does each worker define a manifold for running it inside a dependency engine?
- If a worker is supposed to be active inside an agent, has it been
  wired up to run inside the agent(s)?
- Is a manifold error filter function used so that workers don't
  return dependency engine related errors such as ErrUninstall and
  ErrBounce directly?
    * workers shouldn't be directly concerned the mechnanics of the
      dependency engine
    * Example: https://github.com/juju/juju/blob/master/worker/migrationflag/manifold.go
- Is there a a test for the worker in the featuretests package which
  exercises the key function of the worker?

# CLI & hook tools

Example:

* https://github.com/juju/juju/blob/master/cmd/juju/model/get.go
* https://github.com/juju/juju/blob/master/cmd/juju/model/get_test.go
* https://github.com/juju/juju/blob/master/cmd/juju/model/export_test.go

...

Note that most CLI changes will need to be communicated to QA and Doc teams.

* Did command output, especially yaml and json, change?
* Did command input - arguments, flags - change?
* Was a new command added? Was an old command deleted?

...

- Do command names use the standard, pre-existing verbs and nouns?
    * Reuse the verbs and nouns used by other commands instead of inventing new ones.
    * Unless otherwise approved by the product manager.
- Is a flat command structure used?
    * i.e. `juju add-machine` instead of `juju machine add`
- Is all output done using cmd.Context (not using os.Stdout/Stderr)?
- Are command options (e.g. `--foo`) actually optional?
    * If an option is compulsory then it should probably be positional.
- Are positional arguments generally not optional?
- Is each command’s help summary capitalised and end with a full stop?
- Do command help descriptions use a blank line between paragraphs?
- Do command help descriptions include examples?
- Do command help descriptions include appropriate “See also” references?
- Do command help descriptions read like the help for other commands?
    * Aim for a consistent help style across all commands.
- Do command tests use a mocked out client API facade?
- Is there a a test for the command in the featuretests package which
  exercises the command against the actual API?
