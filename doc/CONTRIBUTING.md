Thanks for your interest in Juju! Contributions like yours make good projects
great.

# TL;DR
- Bug reports should be filed on [Launchpad](https://bugs.launchpad.net/juju/+bugs),
  not GitHub. Please check that your bug has not already been reported.
- When opening a pull request:
  - Check that all your [commits are signed](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits).
  - Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) for commit messages.
  - Check that your patch is [targeting the correct branch](#branches) -
    if not, please rebase it.
  - Please [sign the CLA](#contributor-licence-agreement) if you haven't already.
  - Use the checklist on the [pull request template](../PULL_REQUEST_TEMPLATE.md#checklist)
    to check you haven't forgotten anything.

Contents
========

- [Quick links](#quick-links)
- [Building Juju](#building-juju)
- [Getting started](#getting-started)
- [Dependency management](#dependency-management)
- [Code formatting](#code-formatting)
- [Workflow](#workflow)
   - [Contributor licence agreement](#contributor-licence-agreement)
- [Community](#community)

Quick links
===========

Issue tracker: https://bugs.launchpad.net/juju/+bugs

Documentation:
* https://juju.is/docs
* [source tree docs](doc/)

Community:
* https://chat.charmhub.io
* https://discourse.charmhub.io/

Building Juju
=============

## Installing Go

`juju` is written in [Go](https://go.dev/), a modern, compiled, statically typed,
concurrent language.

Generally, Juju is built against a recent version of Go, with the caveat
that Go versions are not incremented during a release cycle. Check the `go.mod`
file at the root of the project for the targeted version of Go, as this is
authoritative.

For example, the following indicates that Go 1.23 is targeted:

```
module github.com/juju/juju

go 1.23
```

### Official distribution

Go can be [installed](https://golang.org/doc/install#install) from the official
distribution.

## Build Juju and its dependencies

The easiest way to get the Juju source code is to clone the GitHub repository:

    git clone https://github.com/juju/juju.git

To build/install from source, `cd` into the root directory of the cloned repo,
and use `make`.
- `make build` will build the Juju binaries and put them in a
  `_build` subdirectory.
- `make install` will build the Juju binaries and install them in your
  [$GOBIN directory](https://pkg.go.dev/cmd/go#hdr-Compile_and_install_packages_and_dependencies)
  (which defaults to `$GOPATH/bin` or `~/go/bin`).

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

Staying in sync
---------------

Make sure your local copy and GitHub fork stay in sync with upstream:

```bash
cd juju
git pull upstream
```

Dependency management
=====================

go mod
------

Juju uses Go modules to manage dependencies.

Updating dependencies
---------------------

To update a dependency, use the following, ensuring that the dependency is
using a version where possible, or a commit hash if not available:


```
go get -u github.com/the/dependency@v1.2.3
go mod tidy
```

Code formatting
===============

Go provides a tool, `go fmt`, which facilitates a standardized format to go
source code.  The Juju project has one additional policy:

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
        "github.com/juju/loggo/v2"
        gc "gopkg.in/check.v1"

        "github.com/juju/juju/state"
        "github.com/juju/worker/v4"
    )
```

Workflow
========

As a project, Juju follows a specific workflow:

1. sync with upstream
2. create a local feature branch
3. make desired changes
4. test the changes
5. push the feature branch to your github fork
6. reviews
7. auto-merge
8. continuous-integration

Naturally, it is not so linear in practice. Each of these is elaborated below.

Branches
--------

Generally there are multiple versions of Juju in development concurrently,
and so we keep a separate Git branch for each version. When submitting a
patch, please make sure your changes are targeted to the correct branch.

We keep a branch for each minor version of Juju in active development (e.g.
`3.4`, `3.5`) - bug fixes should go into the relevant branch. We also keep a
`main` branch, which will become the next minor version of Juju. All new
features should go into `main`.

If a bug affects multiple Juju versions, please target the **lowest version**
of Juju which is affected. All patches in earlier versions are eventually
"merged through" to later versions.

Creating a new branch
---------------------

All development should be done on a new branch, based on the correct branch
determined above. Pull the latest version of this branch, then create and
checkout a new branch for your changes - e.g. for a patch targeting `main`:

```
git pull upstream main
git checkout -b new_feature main
```

Testing
-------

Some tests may require local lxd to be installed, see
[installing lxd via snap](https://stgraber.org/2016/10/17/lxd-snap-available/).

Juju uses the `gocheck` testing framework, which is automatically installed
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

Many tests use a standalone instance of `mongod` as part of their setup. The
`mongod` binary found in `$PATH` is executed by these suites.  If you don't
already have MongoDB installed, or have difficulty using your installed version
to run Juju tests, you may want to install the [`juju-db` snap](https://snapcraft.io/juju-db), which is guaranteed to work with Juju.

```bash
sudo snap install juju-db --channel 4.4/stable
```

Optionally, you can create aliases for `mongod` and `mongo` to make it easier to
use the snap version:

```bash
sudo snap alias juju-db.mongod mongod
sudo snap alias juju-db.mongo mongo
```

Some tests (particularly those under `./store/...`) assume a MongoDB instance
that supports Javascript for map-reduce functions. These functions are not
supported by `juju-mongodb` and the associated tests will fail unless disabled
with an environment variable:

```bash
JUJU_NOTEST_MONGOJS=1 go test github.com/juju/juju/...
```

Conventional commits
--------------------

Once you have written some code and have tested the changes, the next step is to
`git commit` it. For commit messages Juju follows [conventional commits guidelines](conventional-commits.md).
In short the commits should be of the following form:
```
<type>(optional <scope>): <description>

[optional body]

[optional footer(s)]
```
- **Type:** The type describes the kind of change (e.g., feat, fix, docs, style,
  refactor, test, chore).
- **Scope:** The scope indicates the part of the codebase affected (e.g., model,
  api, cli).
- **Description:** The description briefly describes the change. It should not
  end in any punctuation.
- **Body:** The body should be a detailed explanation of the change. It should
  specify what has been changed and why and include descriptions of the
  behaviour before and after.
- **Footer:**  The footer includes information about breaking changes, issues
  closed, etc.

The type, scope and description should all begin with a lower case letter. None
of the lines can exceed 100 characters in length.

There is a CI action on the Juju GitHub repository that will flag any syntactic
issues with your commit messages. The action is required to pass before code can
be merged so make sure to take a look once your PR is up.

Pushing
-------

When ready for feedback, push your feature branch to github, optionally after
collapsing multiple commits into discrete changes:

```bash
git rebase -i --autosquash main
git push origin new_feature
```

Go to the web page (https://github.com/$YOUR_GITHUB_USERNAME/juju) and hit the
"Pull Request" button, selecting `main` as the target.

This creates a numbered pull request on the github site, where members of the
Juju project can see and comment on the changes.

The title of the PR should match the form of the title of a conventional commit.
This can be the title of the most significant commit in the PR.

Make sure to add a clear description of why and what has been changed, and
include the Launchpad bug number if one exists.

It is often helpful to mention newly created proposals on the Discourse forum,
especially if you would like a specific developer to be aware of the proposal.

Note that updates to your GitHub project will automatically be reflected in
your pull request.

Be sure to have a look at:

https://help.github.com/articles/using-pull-requests


Contributor licence agreement
----------------------

We welcome external contributions to Juju, but in order to incorporate these
into the codebase, we will need you to sign the
[Canonical contributor licence agreement (CLA)](https://ubuntu.com/legal/contributors).
This just gives us permission to use your contributions - you still retain full
copyright of your code.

We have a GitHub Action which checks if you have signed the CLA. To ensure this
passes, please follow these steps:

1. Ensure your Git commits are signed by an email that you can access
   (you can't use the `@users.noreply.github.com` email that GitHub provides).
2. Create an account on [Launchpad](https://launchpad.net/), if you don't
   already have one.
3. Ensure the email you used for Git commits is a **verified** email on your
   Launchpad account. To do this:
   - Go to your Launchpad homepage (`launchpad.net/~[username]`).
   - Check the addresses listed under the **Email** heading. If your Git email
     is listed, you're good.
   - If not, click "Change email settings".
   - Add your Git email as a new address.
   - Follow the instructions to verify your email.
4. Visit the [CLA website](https://ubuntu.com/legal/contributors), scroll down
   and press "Sign the contributor agreement".
5. Read the agreement and fill in your contact details. Ensure that you provide
   your Launchpad username in the "Launchpad id" box.
6. Press "I agree" to sign the CLA.

Eventually, your Launchpad account should be added to the
["Canonical Contributor Agreement" team](https://launchpad.net/~contributor-agreement-canonical).
You will see it listed under "Memberships" on your Launchpad homepage.
Once this happens, the CLA check will pass, and we will happily review
your contribution.


Sanity checking PRs and unit tests
----------------------

All PRs run pre-merge check - unit tests and a small but representative sample
of functional tests. This check is re-run anytime the PR changes, for example
when a new commit is added.

You can also initiate this check by commenting `/build` in the PR.

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
Use the `/merge` comment to land a PR.

Static Analysis
---------------

Static Analysis can be performed by running `make static-analysis`

Required dependencies for full static analysis are:
 - *nix tools (sh, grep etc.)
 - shellcheck
 - python3
 - go
 - golangci-lint

Community
=========

The Juju community is growing and you have a number of options for interacting
beyond the workflow and the [issue tracker](https://bugs.launchpad.net/juju/+bugs).

Use the following links to contact the community:

 - Matrix juju chat: [https://matrix.to/#/#charmhub-juju:ubuntu.com](https://matrix.to/#/#charmhub-juju:ubuntu.com)
 - Matrix juju development chat: [https://matrix.to/#/#charmhub-jujudev:ubuntu.com](https://matrix.to/#/#charmhub-jujudev:ubuntu.com)
 - Discourse forum: [https://discourse.charmhub.io/](https://discourse.charmhub.io/)
