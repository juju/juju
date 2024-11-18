Juju generally has multiple versions in concurrent development, and we keep a separate Git branch for each. Often, a bug
fix or change needs to happen in multiple versions. In this case, we target the fix to the **lowest** relevant version,
and later merge the patch forward into later versions.

For example, for a bug that affects Juju 3.1, 3.2 and 3.3, we target the original fix to the `3.1` branch, then merge
this patch forward into `3.2`, then `3.3`, making changes as needed.

You should make a habit of following up your patches with a forward merge, especially if they are complex changes, or
create merge conflicts.

This document will describe how to do a forward merge. In the following example, we will consider a merge of `2.9` into
`3.1` - but you can replace these with any source and target branch.

1. Ensure your local copies of the source and target branch are up-to-date.
   ```
   git pull 2.9
   git pull 3.1
   ```

2. Create a new merge branch based on the target branch. We suggest giving this a descriptive name such as
   `merge-SRC-TGT-YYYYMMDD`.
   ```
   git checkout -b 'merge-2.9-3.1-20231231' '3.1'
   ```

3. Merge the source branch into your new merge branch.
   ```
   git merge 2.9 -m 'Merge 2.9 into 3.1'
   ```

4. If there are no merge conflicts, the above command will merge the branches and create a merge commit. Skip to step 6.

5. If there are merge conflicts, you will have to resolve these manually. Your IDE might have tools to assist here.
   After resolving conflicts in a file, run `git add <file>` to add it to the index. Then, run `git merge --continue` to
   finish the merge.

5. Push your branch to GitHub and open a new PR to the target branch. In the PR description, please include a list of
   the patches being merged, and list any merge conflicts you encountered. To get
   the PR numbers of the patches in your merge up, use `git log
   upstream/<TARGET-BRANCH-NAME>..upstream/<SOURCE-BRANCH-NAME> --first-parent
   --oneline --no-decorate | sed 's~.*\(#.*\)/.*~- \1~g'`

**Exercise**: write a Bash script to automate the above process.