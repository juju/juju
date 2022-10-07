# GitHub Actions CI

This folder consists of a series of workflows which run on the GitHub repo
(pull requests and the main branches).

There are some workflows that we don't need to run on every PR. For example,
if we are just changing documentation, we don't need to re-run the build tests.
To determine when a workflow runs, we can set `on.pull_request.paths` or
`on.pull_request.paths-ignore` in the yaml definition.

Note that the static analysis checks are marked as "required", so we
**need these to run on every PR** (even if there are no code changes).

Furthermore, we want all the checks to run on the main branch, so please
**don't** set `on.push.paths` or `on.push.paths-ignore` in the workflow yaml.