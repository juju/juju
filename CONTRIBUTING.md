Thanks for your interest in Juju! Contributions like yours make good projects
great.

Contents
========

1. [Building Juju](#building-juju)
2. [Getting started](#getting-started)
3. [Dependency management](#dependency-management)
4. [Code formatting](#code-formatting)
5. [Workflow](#workflow)
6. [Community](#community)

Quick links
===========

Issue tracker: https://bugs.launchpad.net/juju/+bugs

Documentation:
* https://jaas.ai/docs
* [source tree docs](doc/)

Community:
* https://jujucharms.com/community/
* https://discourse.jujucharms.com/
* [#juju on freenode](http://webchat.freenode.net/?channels=juju)

Building Juju
=============

## Installing Go

`juju` is written in Go (http://golang.org), a modern, compiled, statically typed,
concurrent language.

### via snap

    snap install go --channel=1.14/stable --classic

[Snap](https://snapcraft.io/go) is the recommended way to install Go on Linux.

### Other options

See https://golang.org/doc/install#install


## Build Juju and its dependencies

### Download Juju source

    git clone https://github.com/juju/juju.git

Juju does not depend on GOPATH anymore, therefore you can check juju out anywhere.

### Change to the Juju source code directory  

    cd juju

### Install runtime dependencies

    make install-dependencies

### Compile

    make build

### Install

    make install

Getting started
===============

Git
---

Juju uses `git` for version control. To get started, install it and configure
your username:

```bash
git config --global user.name "A. Hacker"
git config --global user.email "a.hacker@example.com"
```

For information on setting up and using `git`, check out the following:

* https://www.atlassian.com/git/tutorials/
* http://git-scm.com/book/en/Getting-Started-Git-Basics
* [GitHub bootcamp](https://help.github.com/categories/54/articles)

GitHub
------

The upstream Juju repository is hosted on [Github](http://github.com). Patches
to Juju are contributed through pull requests (more on that in the
[Pushing](#pushing) section). So you should have a github account and a fork
there. The following steps will help you get that ready:

1. Sign up for GitHub (a free account is fine): https://github.com/join
2. Add your ssh public key to your account: https://github.com/settings/ssh
3. Hit the "Fork" button on the web page for the Juju repo:
    https://github.com/juju/juju

At this point you will have your own copy under your github account. Note
that your fork is not automatically kept in sync with the official Juju repo
(see [Staying in sync](#staying-in-sync)).

Note that Juju has dependencies hosted elsewhere with other version control
tools.

Local clone
-----------

To contribute to Juju you will also need a local clone of your GitHub fork.
The earlier `go get` command will have already cloned the Juju repo for you.
However, that local copy is still set to pull from and push to the upstream
Juju github account. Here is how to fix that (replace <USERNAME> with your
github account name):

```bash
cd juju
git remote set-url origin git@github.com:<USERNAME>/juju.git
```

To simplify staying in sync with upstream, give it a "remote" name:

```bash
git remote add upstream https://github.com/juju/juju.git
```

Add the check script as a git hook:

```bash
cd juju
ln -s scripts/pre-push.bash .git/hooks/pre-push
```

This will ensure that any changes you commit locally pass a basic sanity
check.  Using pre-push requires git 1.8.2 or later, though alternatively
running the check as a pre-commit hook also works.

Staying in sync
---------------

Make sure your local copy and github fork stay in sync with upstream:

```bash
cd juju
git pull upstream develop
git push
```

Dependency management
=====================

In the top-level directory of the Juju repo, there is a file,
[go.mod](go.mod), that holds the revision ids of all the external
projects that Juju depends on. That file is used to freeze the code in
external repositories so that Juju is insulated from changes to those repos.

go mod
------

Juju now uses the built in `go mod` tooling to manage dependencies and therfore
you don't need to do anything to ensure you are building with the correct versions.

Updating dependencies
---------------------

To update a dependency, use `go get -u github.com/the/dependency`.

Code formatting
===============

Go already provides a tool, `go fmt`, that facilitates a standardized
format to go source code.  The Juju project has one additional policy.

Imports
-------

Import statements are grouped into 3 sections: standard library, 3rd party
libraries, juju imports. The tool "go fmt" can be used to ensure each
group is alphabetically sorted. eg:

```go
    import (
        "fmt"
        "time"

        "labix.org/v2/mgo"
        "github.com/juju/loggo"
        gc "gopkg.in/check.v1"

        "github.com/juju/juju/state"
        "github.com/juju/juju/worker"
    )
```

Because "gopkg.in/check.v1" will be referenced frequently in test suites, its
name gets a default short name of just "gc".

Workflow
========

As a project Juju follows a specific workflow:

1. sync with upstream
2. create a local feature branch
3. make desired changes
4. test the changes
5. push the feature branch to your github fork
6. reviews
7. auto-merge
8. continuous-integration

Naturally it is not so linear in practice. Each of these is elaborated below.

Sync with upstream
------------------

First check that the branch is on develop:

```bash
git branch
* develop
  old_feature
```

Then pull in the latest changes from upstream, assuming you have done the setup
as above:

```bash
git pull upstream develop
```

Feature branches
----------------

All development should be done on feature branches based on a current copy of
develop. So after pulling up your local repo, make a new branch for your work:

```bash
git checkout -b new_feature
```

Testing
-------

Some tests may require local lxd to be installed, see
[installing lxd via snap](https://stgraber.org/2016/10/17/lxd-snap-available/).  

Juju uses the `gocheck` testing framework. `gocheck` is automatically installed
as a dependency of `juju`. You can read more about `gocheck` at
http://godoc.org/gopkg.in/check.v1. `gocheck` is integrated into the source of
each package so the standard `go test` command is used to run `gocheck` tests.
For example

```bash
go test github.com/juju/juju/...
```

will run all the tests in the Juju project. By default `gocheck` prints only
minimal output, and as `gocheck` is hooked into the testing framework via a
single `go test` test per package, the usual `go test -v` flags are less
useful. As a replacement the following commands produce more output from
`gocheck`.

```bash
go test -gocheck.v
```

is similar to `go test -v` and outputs the name of each test as it is run as
well as any logging statements. It is important to note that these statements
are buffered until the test completes.

```bash
go test -gocheck.vv
```

extends the previous example by outputting any logging data immediately, rather
than waiting for the test to complete. By default `gocheck` will run all tests
in a package, selected tests can by run by passing `-gocheck.f` to match a
subset of test names.

```bash
go test -gocheck.f '$REGEX'
```

Finally, because by default `go test` runs the tests in the current package,
and is not recursive, the following commands are equal, and will produce no
output.

```bash
cd juju
go test
go test github.com/juju/juju
```

Testing and MongoDB
-------------------

Many tests use a standalone instance of mongod as part of their setup. The
`mongod` binary found in `$PATH` is executed by these suites.

Some tests (particularly those under ./store/...) assume a MongoDB instance
that supports Javascript for map-reduce functions. These functions are not
supported by juju-mongodb and the associated tests will fail unless disabled
with an environment variable:

```bash
JUJU_NOTEST_MONGOJS=1 go test github.com/juju/juju/...
```

Pushing
-------

When ready for feedback, push your feature branch to github, optionally after
collapsing multiple commits into discrete changes:

```bash
git rebase -i --autosquash develop
git push origin new_feature
```

Go to the web page (https://github.com/$YOUR_GITHUB_USERNAME/juju) and hit the
"Pull Request" button, selecting develop as the target.

This creates a numbered pull request on the github site, where members of the
Juju project can see and comment on the changes.

Make sure to add a clear description of why and what has been changed, and
include the Launchpad bug number if one exists.

It is often helpful to mention newly created proposals on the Discourse forum,
especially if you would like a specific developer to be aware of the proposal.

Note that updates to your GitHub project will automatically be reflected in
your pull request.

Be sure to have a look at:

https://help.github.com/articles/using-pull-requests

Sanity checking PRs and unit tests
----------------------

All PRs run pre-merge check - unit tests and a small but representative sample
of functional tests. This check is re-run anytime the PR changes, for example
when a new commit is added.

You can also initiate this check by commenting !!build!! in the PR.

Code review
-----------

The Juju project uses peer review of pull requests prior to merging to
facilitate improvements both in code quality and in design.

Once you have created your pull request, it will be reviewed. Make sure to
address the feedback. Your request might go through several rounds of feedback
before the patch is approved or rejected. Once you get an approval from a
member of the Juju project, you are ready to have your patch merged.
Congratulations!

Continuous integration
----------------------

Continuous integration is automated through Jenkins:

The bot runs on all commits during the PRE process, as well as handles merges.
Use the `$$merge$$` comment to land a PR.

Static Analysis
---------------

Static Analysis can be performed by running `make static-analysis`

Required dependencies for full static analysis are:
 - *nix tools (sh, grep etc.)
 - shellcheck
 - python3
 - go
 - golint
 - goimports
 - deadcode
 - misspell
 - unconvert
 - ineffassign

Community
=========

The Juju community is growing and you have a number of options for interacting
beyond the workflow and the [issue tracker](https://bugs.launchpad.net/juju/+bugs).

Use the following links to contact the community:

 - Community page: https://jujucharms.com/community/
 - IRC channel on freenode: `#juju`
 - Discourse forum: [https://discourse.jujucharms.com/](https://discourse.jujucharms.com/)
