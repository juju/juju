Contents
========

1. [Getting started](#getting-started)
2. [Dependency management](#dependency-management)
3. [Code formatting](#code-formatting)
4. [Workflow](#workflow)
5. [Community](#community)


Quick links
===========

* issue tracker: https://launchpad.net/juju

Documentation:
* https://jujucharms.com/docs/
* overview: http://blog.labix.org/2013/06/25/the-heart-of-juju
* [other docs](doc/)

Community:
* https://jujucharms.com/community/
* juju: https://lists.ubuntu.com/mailman/listinfo/juju
* juju-dev: https://lists.ubuntu.com/mailman/listinfo/juju-dev
* [#juju on freenode](http://webchat.freenode.net/?channels=juju)
* [#juju-dev on freenode](http://webchat.freenode.net/?channels=juju-dev)


Getting started
===============

Thanks for contributing to `juju`!  Contributions like yours make good
projects great.  Before contributing to `juju` please read the following
sections describing the tools and conventions of this project.  This
file is a companion to [README](README.md) and it is assumed that file
has been read and followed prior.

Specifically, the following commands should already have been run:

```shell
$ go get -d -v github.com/juju/juju/...
$ cd $GOPATH/src/github.com/juju/juju
$ make install-dependencies
```

The `-d` option means the source (for juju and its dependencies) is only
downloaded and not built.  This is required since the dependencies may be
out of sync and fail to build properly.  See the
[Dependency management](#dependency-management)
section for more information.

Git
---

Juju uses git for source control.  To get started, install git and
configure your username:

```shell
$ git config --global user.name "A. Hacker"
$ git config --global user.email "a.hacker@example.com"
```

For information on setting up and using git, check out the following:

* https://www.atlassian.com/git/tutorials/
* http://git-scm.com/book/en/Getting-Started-Git-Basics
* [Github bootcamp](https://help.github.com/categories/54/articles)

Github
------

The upstream juju repository is hosted on [Github](http://github.com).
Patches to juju are contributed through pull requests (more on that in the
[Pushing](#pushing) section).  So you should have a github account and
a fork there.  The following steps will help you get that ready:

1. Sign up for github (a free account is fine):  https://github.com/join
2. Add your ssh public key to your account:  https://github.com/settings/ssh
3. Hit the "Fork" button on the web page for the juju repo:
    https://github.com/juju/juju

At this point you will have your own copy under your github account.  Note
that your fork is not automatically kept in sync with the official juju repo
(see [Staying in sync](#staying-in-sync)).

Note that juju has dependencies hosted elsewhere with other version
control tools.

Local clone
-----------

To contribute to juju you will also need a local clone of your github fork.
The earlier `go get` command will have already cloned the juju repo for you.
However, that local copy is still set to pull from and push to the upstream
juju github account.  Here is how to fix that (replace <USERNAME> with your
github account name):

```shell
$ cd $GOPATH/src/github.com/juju/juju
$ git remote set-url origin git@github.com:<USERNAME>/juju.git
```

To simplify staying in sync with upstream, give it a "remote" name:

```shell
$ git remote add upstream https://github.com/juju/juju.git
```

Add the check script as a git hook:

```shell
$ cd $GOPATH/src/github.com/juju/juju
$ ln -s scripts/pre-push.bash .git/hooks/pre-push
```

This will ensure that any changes you commit locally pass a basic sanity
check.  Using pre-push requires git 1.8.2 or later, though alternatively
running the check as a pre-commit hook also works.

Staying in sync
---------------

Make sure your local copy and github fork stay in sync with upstream:

```shell
$ cd $GOPATH/src/github.com/juju/juju
$ git pull upstream develop
$ git push
```


Dependency management
=====================

In the top-level directory of the juju repo, there is a file,
[Gopkg.lock](Gopkg.lock), that holds the revision ids of all
the external projects that juju depends on.  That file is used to freeze
the code in external repositories so that juju is insulated from changes
to those repos.

dep
------

[dep](https://golang.github.io/dep) is the tool that does the freezing.
After getting the juju code, you need to get `dep`:

```shell
go get github.com/golang/dep
```

This installs the `dep` application.  You can then run `dep` from the
root of juju, to set the revision number on the external repositories:

```shell
cd $GOPATH/src/github.com/juju/juju
make dep
```

Now you are ready to build, run, test, etc.

Staying up-to-date
------------------

The [Gopkg.lock](Gopkg.lock) file can get out of date, for
example when you have changed Gopkg.toml. When it is out of date, run
`dep`. In practice, you can wait until you get a compile error about
an external package not existing/having an incorrect API, and then rerun
`dep ensure -v`.

Updating dependencies
---------------------

If you update a repo that juju depends on, you will need to recreate
`Gopkg.lock`:

```shell
make rebuild-dependencies [dep-update=true]
```


Code formatting
===============

Go already provides a tool, `go fmt`, that facilitates a standardized
format to go source code.  The juju project has one additional policy.

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

Because "gopkg.in/check.v1" will be referenced frequently in test suites,
its name gets a default short name of just "gc".


Workflow
========

As a project juju follows a specific workflow:

1. sync with upstream
2. create a local feature branch
3. make desired changes
4. test the changes
5. push the feature branch to your github fork
6. reviews
7. auto-merge
8. continuous-integration

Naturally it is not so linear in practice.  Each of these is elaborated below.

Sync with upstream
------------------

First check that the branch is on develop:

```shell
$ git branch
* develop
  old_feature
```

Then pull in the latest changes from upstream, assuming you have done
the setup as above:

```shell
$ git pull upstream develop
```

Feature branches
----------------

All development should be done on feature branches based on a current
copy of develop.  So after pulling up your local repo, make a new branch
for your work:

```shell
$ git checkout -b new_feature
```

Testing
-------

`juju` uses the `gocheck` testing framework. `gocheck` is automatically
installed as a dependency of `juju`. You can read more about `gocheck`
at http://godoc.org/gopkg.in/check.v1. `gocheck` is integrated
into the source of each package so the standard `go test` command is used
to run `gocheck` tests. For example

```shell
$ go test github.com/juju/juju/...
```
will run all the tests in the `juju` project. By default `gocheck` prints
only minimal output, and as `gocheck` is hooked into the testing framework via
a single `go test` test per package, the usual `go test -v` flags are less
useful. As a replacement the following commands produce more output from
`gocheck`.

```shell
$ go test -gocheck.v
```

is similar to `go test -v` and outputs the name of each test as it is run as
well as any logging statements. It is important to note that these statements
are buffered until the test completes.

```shell
$ go test -gocheck.vv
```

extends the previous example by outputting any logging data immediately, rather
than waiting for the test to complete. By default `gocheck` will run all tests
in a package, selected tests can by run by passing `-gocheck.f` to match a subset of test names.


```shell
$ go test -gocheck.f '$REGEX'
```

Finally, because by default `go test` runs the tests in the current package, and
is not recursive, the following commands are equal, and will produce no output.

```shell
$ cd $GOPATH/src/github.com/juju/juju
$ go test
$ go test github.com/juju/juju
```

Testing and MongoDB
-------------------

Many tests use a standalone instance of mongod as part of their setup. The
`mongod` binary found in `$PATH` is executed by these suites.

Some tests (particularly those under ./store/...) assume a MongoDB instance
that supports Javascript for map-reduce functions. These functions are not
supported by juju-mongodb and the associated tests will fail unless disabled
with an environment variable:

```shell
$ JUJU_NOTEST_MONGOJS=1 go test github.com/juju/juju/...
```

Pushing
-------

When ready for feedback, push your feature branch to github, optionally after
collapsing multiple commits into discrete changes:

```shell
$ git rebase -i --autosquash develop
$ git push origin new_feature
```

Go to the web page (https://github.com/$YOUR_GITHUB_USERNAME/juju)
and hit the "Pull Request" button, selecting develop as the target.

This creates a numbered pull request on the github site, where members
of the juju project can see and comment on the changes.

Make sure to add a clear description of why and what has been changed,
and include the launchpad bug number if one exists.

It is often helpful to mention newly created proposals in the #juju-dev
IRC channel on Freenode, especially if you would like a specific developer
to be aware of the proposal.

Note that updates to your github project will automatically be reflected in
your pull request.

Be sure to have a look at:

https://help.github.com/articles/using-pull-requests

Sanity checking PRs and unit tests
----------------------

All PRs run pre-merge check - unit tests and a small but representative sample of 
functional tests. This check is re-run anytime the PR changes, for example when 
a new commit is added.

You can also initiate this check by commenting !!build!! on the PR.

Code review
-----------

The juju project uses peer review of pull requests prior to merging to
facilitate improvements both in code quality and in design.

Once you have created your pull request, it will be reviewed.  Make sure
to address the feedback.  Your request might go through several rounds
of feedback before the patch is approved or rejected.  Once you get an approval
from a member of the juju project, you are ready to have your patch merged.
Congratulations!


Continuous integration
----------------------

Continuous integration is automated through Jenkins:

The bot runs on all commits during the PRE process,
as well as handles merges. Use the `$$merge$$` comment to land PR's.


Static Analysis
---------------

Static Analysis of the code is provided by [gometalinter](https://github.com/alecthomas/gometalinter).

The Static Analysis runs every linter in parallel over the juju code base with
the aim to spot any potential errors that should be resolved. As a default all
the linters are disabled and only a selection of linters are run on each pass.
The linters that are run, are known to pass with the state of the current code
base.

You can independantly run the linters on the code base to validate any issues
before committing or pushing using the following command from the root of the
repo:

```shell
./scripts/gometalinter.bash
```

If you already have the git hook installed for pre-pushing, then the linters
will also run during that check.

Adding new linters can be done, by editing the `./scripts/gometalinter.bash`
file to also including/removing the desired linters.

Additionally to turn off the linter check for pushing or verifying then
setting the environment variable `IGNORE_GOMETALINTER` before hand will cause
this to happen.


Community
=========

The juju community is growing and you have a number of options for
interacting beyond the workflow and the
[issue tracker](https://launchpad.net/juju).

Take a look at the community page:

 https://jujucharms.com/community/

juju has a channel on IRC (freenode.net):

* `#juju`

Juju Discourse:

* [https://discourse.jujucharms.com/](https://discourse.jujucharms.com/)
