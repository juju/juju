Contributing to goose
=====================

We encourage everyone who is familiar with the [OpenStack
API](http://developer.openstack.org/api-ref.html) and is willing to
support and improve the project to become a contributor. Current list
of official maintainers can be found on the [go-goose People](https://github.com/orgs/go-goose/people)
list. Current and past contributors list is in the [AUTHORS.md](AUTHORS.md) file.

This file contains instructions and guidelines for contributors.

Code of conduct
---------------

We are committed to providing a friendly, safe and welcoming environment
for all users and community members. Please, review the [Ubuntu Code of Conduct](https://launchpad.net/codeofconduct/1.1),
which covers the most important aspects we expect from contributors:

 * Be considerate - treat others nicely, everybody can provide valuable contributions.
 * Be respectful of others, even if you don't agree with them. Keep an open mind.
 * Be collaborative - this is essential in an open source community.
 * When we disagree, we consult others - it's important to solve disagreements constructively.
 * When unsure, ask for help.
 * Step down considerately - if you need to leave the project, minimize disruption.

Ways to contribute
------------------

There are several ways to contribute to the project:

 * Report [issues](https://github.com/go-goose/goose/issues/new) you might have.
 * Please, make sure there is no existing [known issue](https://github.com/go-goose/goose/issues) when reporting a new one.
 * Propose a patch or a bug fix by opening a [pull request](https://help.github.com/articles/creating-a-pull-request/). Check GitHub help on [how to collaborate](https://help.github.com/categories/collaborating/).
 * Give feedback for [known issues](https://github.com/go-goose/goose/issues/) or proposed [pull requests](https://github.com/go-goose/goose/pulls).

For some of those things you will need a [GitHub account](https://github.com/signup/free), if you don't have one.

Contributing a patch
--------------------

Found a bug or want to suggest an improvement?
Great! Here are the steps anyone can follow to propose a bug fix or patch.

 * [Fork](https://help.github.com/articles/fork-a-repo/) the [`go-goose/goose`](https://github.com/go-goose/goose) repository.
 * If you think you found a bug, please check the existing [issues](https://github.com/go-goose/goose/issues) to see if it's a known problem.
 * Otherwise, [open a new issue](https://github.com/go-goose/goose/issues/new) for it.
 * Clone your forked repository locally:
```
$ git clone https://github.com/<your-github-username>/goose
```
 * For the unit tests, you will need [gocheck.v1](https://github.com/go-check/check):
```
$ go get gopkg.in/check.v1
```
 * Create a feature branch for your contribution. Make your changes there. 
 * It's recommended to try keeping your changes as small as possible.
 * Split bigger changes in several pull request to make the code review easier.
 * Be sure to write tests for your code changes and run them before proposing:
```
$ go test -check.v
```
 * Push your feature branch to your fork.
 * Open a pull request with a description of your change.
 * A maintainer should notice your pull request and do a code review. You can also ask a [maintainer](https://github.com/orgs/go-goose/people) for review.
 * Reply to comments, fix issues, push your changes. 
 * Depending on the size of the patch, this process can be repeated a few times.
 * All merges are gated on running the full test suite, so make sure to run all the tests locally.
 * Once you get an approval, ask a maintainer to merge your patch.

Becoming an official maintainer
-------------------------------

Thanks for considering becoming a maintainer of goose! It's not
required to be a maintainer to contribute, but if you find yourself
frequently proposing patches and can dedicate some of your time to
help, please consider following the following procedure.

 * You need a [GitHub account](https://github.com/signup/free) if you don't have one.
 * Review and sign the Canonical [Contributor License Agreement](http://www.ubuntu.com/legal/contributors/).
 * For any questions about the CLA, you might find the [CLA FAQ](http://www.ubuntu.com/legal/contributors/licence-agreement-faq) page useful.
 * Request to become a maintainer by contacting the [existing maintainers](https://github.com/orgs/go-goose/people).
 * You're welcome to add your name to the [AUTHORS.md](AUTHORS.md) list once approved.

General guidelines
------------------

The following list is not exhaustive or in any particular order. It
providers things to keep in mind when contributing to goose. Be
reasonable and considerate and please ask for help, if something is
not clear.

 * Commit early, commit often.
 * Use `git rebase` before proposing your changes to squash minor commits. Let's keep the commit log cleaner.
 * **Do not** rebase commits you already pushed, even when in your own fork. Others might depend on them.
 * Write new tests and update existing ones when changing the code. All changes should have tests, when possible.
 * Use `go fmt` to format your code before pushing.
 * Document exported types, functions, constants, variables, etc.
 * See the excellent [Effective Go](http://golang.org/doc/effective_go.html) style guide, which we use.
 * When reporting issues, provide the necessary information to reproduce the issue.

Thanks for your interest in goose!
