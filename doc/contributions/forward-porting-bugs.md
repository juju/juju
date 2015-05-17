juju and Forward Porting Bug Fixes
==================================

When fixing a bug that affects more than a single branch the prefered
process for fixing the bug is to perform the work against the oldest
branch and then forward port that bug back up to each release branch
and finally in to master.

For example, if a bug is reported to impact R1, R2, and master. You
would perform the work against a branch of R1. When your fix is ready
you would use the pull request and review process. See: [reviewboard]
Then you would forward port that fix to R2, and finally once your fix
was merged in to R2, you would forward port the fix to current master.

How To Forward Port
===================

Once your inital fix has been merged by the bot the process of forward
porting can be done with a few git commands. In this example I will show
forward porting the fix in to master.

You will want to locate the SHA for the merge commit that was generated
by jujubot. This will be viewable on github or in your git log output.
Copy the SHA since we will use it to cherry-pick the fix in to master.

    git checkout master
    git checkout -b <fixed-branch-name>
    git cherry-pick -m 1 <merge-commit-sha>

You may have some minor merge conflicts with the cherry-pick that need
to be fixed, this is rare when forwarding porting, but occasionally it
does happen.

    git push your-remote <fixed-branch-name>

Now your new branch is ready to follow the same pull request and review
process as the original fix. Be sure to note that this is a forward port
and link to the previous review.
