juju and Code Review
====================

The `juju` project uses ReviewBoard for code review, falling back to
github in the event that ReviewBoard is not available.  The review site
is found at:

 http://reviews.vapour.ws/

Authentication
--------------

The site uses github OAuth for authentication.  To log in simply go to
login page and click the "github" button.  The first time you do this,
it will redirect you to github to approve access and then redirect you
back.  This first time is the only one where you will be redirected to
github.  Furthermore, ReviewBoard will keep you logged in between visits
via session cookies.

That first time you log in, a ReviewBoard account will be created for
you using your github username.  However, no other account information
(including email address) is added for you.

All OAuth login capability is provided by a ReviewBoard plugin (via a
charm):

 https://pypi.python.org/pypi?name=rb_oauth&:action=display

Email Notifications
-------------------

As noted, your email address is not automatically added to your
reviewboard account.  If you want to receive review-related email, be
sure to add your email address to your ReviewBoard profile.

Github Integration
------------------

We have webhooks set up on our Github repos that trigger on pull request
events.  The webhooks send the pull request data to the ReviewBoard site
which does the following for new pull requests:

1. create a new review request for the PR
2. add a link to the PR on the review request 
3. add a link to the review request on the PR body

When a pull request is updated, the review request is updated with the
latest diff.  When a PR is closed, the corresponding review request is
closed.  Likewise if a PR is re-opened.

The Github integration means that for normal review workflow, the only
required tool is our ReviewBoard site.

All ReviewBoard-Github integration is provided by a ReviewBoard plugin
(without a charm):

 https://bitbucket.org/ericsnowcurrently/rb_webhooks_extension


Manual Review Workflow
======================

In cases where the github-reviewboard integration does not work, some or
all of these manual steps will be necessary for review workflow.

Requirements
------------

Some manual workflow steps require the `rbt` commandline tool.  More
details on installation and usage are found below.

.reviewboardrc
--------------

`rbt` requires a .reviewboardrc file at the top of the repo on which you
are working.  Some of the juju repos have this file committed.  For
those that do not, you will need to run `rbt setup-repo` to generate
one.  Alternately copy the same file from a repo that has it and update
"REPOSITORY" to the label for the current repo in ReviewBoard.  In
either case, the file should also be committed and merged upstream.
This has already been done for at least some of the key repos.

If you followed the recommendation from CONTRIBUTING.md on git "remotes"
(which you should have), you will need to make sure you have the
following in the .reviewboardrc file:

 TRACKING_BRANCH = "upstream/master"

Otherwise rbt will generate incorrect diffs.

Manually Creating New Review Requests
-------------------------------------

Once you have logged in to ReviewBoard for the first time you are ready
to create new review requests.  Each review request should be associated
with a pull request on github.  So after your pull request is created,
follow these steps:

1. run "rbt post" (see more info below on RBTools)
2. follow the link and hit the "publish" button (or use the "-p" option)
3. add a comment to the PR with a link to the review request

At this point your review request should get reviewed.  Make sure to
address the feedback.  Your request might go through several rounds of
feedback before the patch is approved or rejected.

Manually Updating an Existing Review Request
--------------------------------------------

To update an existing pull request:

1. push your updated branch to your github clone (this will
   automatically update the pull request)
2. run "rbt post -u" or "rbt post -r #"
3. hit the "publish" button (or use the "-p" option)

Important: Make sure you use one of those two options.  Otherwise there
is a good chance that ReviewBoard will create a new review request,
which is a problem because revisions are linked to review requests
(even discarded ones).  So the accidental review request would prevent
you from updating the correct one after that.  If that happens you will
need to get one of the ReviewBoard admins to delete the accidental
review request.  Considering the overhead involved for everyone, it
would be better just make sure you always use "-u" or "-r" for updates.


ReviewBoard
===========

ReviewBoard is an open-source code review tool that provides a full-
featured web interface, an extensive remote API, and a command-line
client.  Furthermore, it is easy to install and highly extensible
through a plugin framework.

For more information see https://www.reviewboard.org/docs/.

RBTools (rbt)
-----------------------------

`rbt` is the command-line client to the ReviewBoard remote API.  You
will need to install it before you can use it.  The documentation for
the tools provides instructions.

  https://www.reviewboard.org/docs/rbtools/0.6/

While `rbt` provides different functionality through various
subcommands, the main case is creating and updating review requests.
This is done via `rbt post`.

The rbtools documentation includes information on various helpful
command-line options.  These include:

* automatically publish the draft request/update: rbt post -p
* automatically open the request in your browser: rbt post -o

For best results when using `rbt`:

* make sure you are on the branch that matches the PR for which you are
  creating a review request
* make sure that branch is based on an up-to-date master

rbt Authentication
------------------

The first time you use `rbt`, rbtools will request your ReviewBoard
credentials.  Since our ReviewBoard users do not have passwords you must
trigger OAuth authentication:

  username: `<github username>`
  password: `oauth:<github username>@github`

Other Features
--------------

`rbt post --parent` allows you to chain review requests.
