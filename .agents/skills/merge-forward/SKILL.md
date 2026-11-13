---
name: merge-forward
description: "Perform a Juju merge-forward between two repository branches using the standard workflow: verify repo state, fetch both branches, create a merge branch from the target tip, merge the source branch, identify conflict-causing PRs on both sides, optionally resolve conflicts, and prepare the PR description. Use when asked to \"merge forward\", \"merge X into Y\", or \"do a merge-forward from X to Y\" in the juju repository."
---

# Merge Forward

Juju has multiple versions in concurrent development, each on its own
branch. Fixes target the lowest relevant version and are later merged
forward into every later version: 4.0 into 4.1, 4.1 into main. The
canonical procedure is: sync both branches, create a merge branch off
the target tip, merge the source, resolve conflicts, push the merge
branch, and open a PR listing the included patches and any conflicts
encountered. This skill automates that procedure.

Parse `SOURCE` and `TARGET` branch names from the request. If either is
missing, stop and report the error.

## Unattended Operation

This skill runs unattended: there is no user to answer questions. Never ask
for confirmation at any step and never wait for input. Decide, continue, and
record anything that needs human review in the PR description.

## Pre-flight Checks

Run these checks and stop with a clear error if any fails.

### Clean Working Tree

Run:

```bash
git status --porcelain
```

If the output is non-empty, stop and output which files are modified or
staged so they can be stashed or committed before proceeding.

### Upstream Remote

Run:

```bash
git remote get-url upstream
```

The URL must contain `github.com/juju/juju`. If the remote is missing or
points elsewhere, stop and explain.

### Branch Existence

Run:

```bash
git ls-remote --heads upstream SOURCE
git ls-remote --heads upstream TARGET
```

If either returns empty, stop and name which branch was not found.

## Fetch Both Branches

Fetch only. Do not touch any local branch.

```bash
git fetch upstream SOURCE
git fetch upstream TARGET
```

After fetching, report the tip SHAs:

```bash
git rev-parse --short upstream/SOURCE
git rev-parse --short upstream/TARGET
```

## Create The Merge Branch

Branch off the target tip, not the current `HEAD`.

```bash
MERGE_BRANCH="merge/SOURCE-TARGET-$(date +%Y%m%d-%H%M%S)"
git checkout -b "$MERGE_BRANCH" upstream/TARGET
```

The timestamp makes the branch name unique per run, so a new PR can be
opened the same day once the previous one is merged or closed. Never
reuse an existing branch name.

Confirm with the output:
`Created branch $MERGE_BRANCH from upstream/TARGET (<sha>)`.

## Execute The Merge

Run:

```bash
git merge upstream/SOURCE -m "chore: merge SOURCE into TARGET"
```

- Exit `0`: continue to PR description generation.
- Non-zero: continue to conflict handling.

## Handle Conflicts

### List Conflicted Files

Run:

```bash
git diff --name-only --diff-filter=U
```

Output the conflicted file list.

### Identify Responsible PRs Per File

Compute the merge-base once:

```bash
MERGE_BASE=$(git merge-base upstream/TARGET upstream/SOURCE)
```

For each conflicted file `FILE`, run:

```bash
git log "$MERGE_BASE..upstream/SOURCE" --merges --oneline -- FILE
git log "$MERGE_BASE..upstream/TARGET" --merges --oneline -- FILE
```

If either query returns no merge commits, fall back to all commits:

```bash
git log "$MERGE_BASE..upstream/SOURCE" --oneline -- FILE
git log "$MERGE_BASE..upstream/TARGET" --oneline -- FILE
```

Report per file:

```text
Conflicts:
  FILE
    Source (SOURCE) PRs: #XXXXX, #YYYYY, ...
    Target (TARGET) PRs: #AAAAA, #BBBBB, ...
```

### Resolve Without Asking

Do not ask whether to resolve the conflicts. Attempt to resolve every
conflicted file yourself and continue.

### Resolve Each Conflict

For each conflicted file:

1. Read the full conflicted file.
2. Read each side:

   ```bash
   git show upstream/SOURCE:FILE
   git show upstream/TARGET:FILE
   ```

3. Read both diffs from the merge-base:

   ```bash
   git diff "$MERGE_BASE" upstream/SOURCE -- FILE
   git diff "$MERGE_BASE" upstream/TARGET -- FILE
   ```

4. Apply a resolution that preserves both changes where possible.
5. If the changes are genuinely incompatible, keep both sides of the
   hunk (a union) rather than dropping one side. Never silently take only
   the TARGET side: development happens on prior versions too. Mark the
   file as an incompatible merge in the PR description and record a risk
   and confidence score for the resolution there.
6. Remove all conflict markers and check with `git diff --check $FILE` and
   stage the file:

   ```bash
   git diff --check FILE
   git add FILE
   ```

After all files are resolved, finalize the merge:

```bash
git merge --continue
```

In unattended runs, set `GIT_EDITOR=true` first so no editor opens.

If `--continue` fails, use `git diff --check` until it passes cleanly.

## Post-Merge Cleanup

After the merge commit is created, remove the stale schema DDL snapshot
files carried over from the source series; they are regenerated per
release and must not accumulate on the target branch:

```bash
rm -f domain/schema/controller/SOURCE.*.ddl
rm -f domain/schema/model/SOURCE.*.ddl
```

For example, for SOURCE=4.0:

```bash
rm -f domain/schema/controller/4.0.*.ddl
rm -f domain/schema/model/4.0.*.ddl
```

Commit the removals as a separate commit on the merge branch and mention
them in the PR description.

## Generate The PR Description

### List Included PRs

Run:

```bash
git log upstream/TARGET..HEAD --merges --oneline \
  | grep "pull request" \
  | sed 's/.*Merge pull request //' \
  | sed -r 's/from (.*)\/.*/@\1/'
```

This produces lines like `#22345 @username`.

Optionally enrich each PR with title and author:

```bash
gh pr view NUMBER --repo juju/juju --json title,author \
  --jq '"\(.title) — @\(.author.login)"'
```

### Output The PR Description

Output the following. Do not post it anywhere automatically.

```text
## Summary

Merge-forward: SOURCE → TARGET

### Automation

- CI run: $GITHUB_RUN_URL
- Model: $OPENCODE_MODEL

### Included PRs

- #XXXXX @author — <title>
- #YYYYY @author — <title>
...

### Conflicts resolved

No conflicts — clean merge.
```

The Automation section is the audit trail. Populate `GITHUB_RUN_URL` and
`OPENCODE_MODEL` from the environment in CI runs; omit the section in
interactive runs where they are unset.

If conflicts were resolved, replace that section with:

```text
- `FILE`
  - Source (#SOURCE-PR) vs Target (#TARGET-PR)
  - Resolution: <one-line description>
  - Risk: low | medium | high — <short reason>
  - Confidence: low | medium | high — <short reason>
```

Use only `low`, `medium`, or `high` for risk and confidence so reviewers
can compare resolutions consistently.

## Publish The Merge

### Interactive

Do not push automatically. Output:

```text
Branch $MERGE_BRANCH is ready. To open the PR:

  git push origin $MERGE_BRANCH

Then open a pull request from $MERGE_BRANCH → TARGET on GitHub.
```

### CI (e.g. `GITHUB_ACTIONS` is set)

There is no user to hand off to. Publish the merge yourself:

1. Push only the merge branch:

   ```bash
   git push upstream "$MERGE_BRANCH"
   ```

2. Open the pull request, using the PR description above as the body:

   ```bash
   gh pr create --base TARGET --head "$MERGE_BRANCH" \
     --title "chore: merge SOURCE into TARGET" --body-file BODY_FILE
   ```

3. Assign up to 3 unique reviewers, chosen by their involvement in the
   merge. Aim for at least 2 when enough eligible candidates exist, but
   continue if fewer than 2 can be assigned.

   GitHub may auto-request reviewers (e.g. from CODEOWNERS) when the PR
   is created. Remove all of them first, so only the reviewers chosen
   below remain:

   ```bash
   PR_NUMBER=$(gh pr view PR_URL --json number --jq .number)

   AUTO_REVIEWERS=$(gh api \
     "repos/juju/juju/pulls/$PR_NUMBER/requested_reviewers")
   mapfile -t AUTO_USERS < <(
     jq -r '.users[].login' <<< "$AUTO_REVIEWERS"
   )
   mapfile -t AUTO_TEAMS < <(
     jq -r '.teams[].slug' <<< "$AUTO_REVIEWERS"
   )

   args=()
   for login in ${AUTO_USERS[@]+"${AUTO_USERS[@]}"}; do
     args+=(-f "reviewers[]=$login")
   done
   for slug in ${AUTO_TEAMS[@]+"${AUTO_TEAMS[@]}"}; do
     args+=(-f "team_reviewers[]=$slug")
   done
   if ((${#args[@]})); then
     gh api "repos/juju/juju/pulls/$PR_NUMBER/requested_reviewers" \
       --method DELETE "${args[@]}"
   fi
   ```

   Build an ordered candidate list as follows:

   - First, add authors of the PRs that caused conflicts, ordered by how
     many conflicted files their PRs touch.
   - Then inspect every included PR and add its author. Rank these
     remaining authors by the sum of additions and deletions across all
     their included PRs, highest first:

     ```bash
     gh pr view NUMBER --repo juju/juju \
       --json author,additions,deletions \
       --jq '"\(.author.login) \(.additions + .deletions)"'
     ```

   Deduplicate candidates by login while preserving that order. Determine
   the new PR author, then exclude it along with bot accounts (logins ending
   in `[bot]`):

   ```bash
   PR_AUTHOR=$(gh pr view PR_URL --json author --jq .author.login)
   ```

   Request candidates one at a time against the URL printed by
   `gh pr create`. Continue down the list until 3 reviewers have been
   assigned or no candidates remain. If GitHub rejects a candidate, for
   example because they are not a collaborator, continue with the next
   candidate.

   ```bash
   gh pr edit PR_URL --add-reviewer LOGIN
   ```

   Finally, query GitHub and list the reviewers that were actually
   requested, rather than relying on command output:

   ```bash
   gh pr view PR_URL --json reviewRequests \
     --jq '.reviewRequests[].login'
   ```

Never push to SOURCE or TARGET directly. The only refs that may be pushed
are `merge/SOURCE-TARGET-YYYYMMDD-HHMMSS` branches. Do not commit or stage
anything unrelated to the merge.

## Feedback Loop

This skill drives unattended merges, so it must stay honest about what
happens in runs:

- If the agent detects a bad resolution, stale instruction, or failing
  command during the run, it records the finding in the merge PR
  description. It must not edit this skill from the merge branch.
- If reviewers or CI detect a problem afterwards, a human records it on
  the merge PR and follows up with a change to this skill, so the next
  run does not repeat the failure.

A broken merge must never become a recurring manual repair: fix the skill
instead.

## Conventions

- Merge branch name: `merge/SOURCE-TARGET-YYYYMMDD-HHMMSS` (unique per
  run, so a new PR can be opened the same day once the previous one is
  merged or closed)
- Merge commit message: `chore: merge SOURCE into TARGET`
- `upstream` remote: `github.com/juju/juju`
- Reviewers: up to 3, preferably at least 2 when enough eligible
  candidates exist, chosen by merge involvement (authors of
  conflict-causing PRs first, then authors of the largest changes);
  reviewers auto-assigned by GitHub are removed first
- The workflow deletes a pair's `merge/*` branches once all their PRs
  are merged or closed, or when no PR exists
