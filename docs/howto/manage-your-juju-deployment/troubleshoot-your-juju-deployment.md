---
myst:
  html_meta:
    description: "Troubleshoot Juju deployment issues: diagnose failures, check logs, analyze status, and resolve common errors in controllers and models."
---

(troubleshoot-your-deployment)=
# Troubleshoot your Juju deployment

From the point of view of the user, there are four basic failure scenarios:

1. Command that fails to return – things hang at some step (e.g., `bootstrap` or `deploy`) and eventually timeout with an error.
1. Command that returns an error.
1. Command that returns but, immediately after, `juju status` shows errors.
1. Things look fine but, at some later point, `juju status` shows errors.

In all cases you'll want to understand what's causing the error so you can figure out the way out:

- For (1)-(3) you can check the documentation for the specific procedure you were trying to perform right before the error. The how-to for the procedure might have a troubleshooting box, and the reference page of the thing you were changing lists the errors of each of its operations, with the message, what triggers it and how to solve the issue.

```{ibnote}
See more:

- The troubleshooting box at the end of {ref}`bootstrap-a-controller`
- The errors of {ref}`adding models <model-adding>`, {ref}`migrating models <model-migrating>` and {ref}`destroying models <model-destroying>`
- The errors of {ref}`adding clouds <cloud-adding>`, {ref}`removing clouds <cloud-removing>` and {ref}`adding SSH keys <ssh-key-adding>`
```

- For (1)-(2) you can also retry the command with the global flags `--debug` and `--verbose` (best used together; for `bootstrap`, also use `--keep-broken` -- if a machine is provisioned, this will ensure that it is not destroyed upon bootstrap fail, which will enable you to examine the logs).
- For all of (1)-(4), you can examine the logs by
    - running `juju debug-log` (best used with `--tail`, because some errors are transient so the last lines tend to be the most relevant; also with  `–level=ERROR` and, if the point of failure is known, `–include ...` as well, to filter the output) or
    - examining the log files directly.

```{ibnote}
See more: {ref}`command-juju-debug-log`, {ref}`log`, {ref}`manage-logs`
```

- For (3)-(4) the error might also be coming from a particular hook or action. In that case, use `juju debug-hooks` to launch a tmux session that will intercept matching hooks and/or actions. Then you can fix the error by manually configuring the workload, or editing the charm code. Once it is fixed you can run `juju resolved` to inform the charm that you have fixed the issue and it can continue.

```{ibnote}
See more: {ref}`command-juju-debug-hooks`, {ref}`command-juju-resolved`
```

If none of this helps, use the information you've gathered to ask for help on our public [Charmhub Matrix chat](https://matrix.to/#/#charmhub:ubuntu.com) or our public [Charmhub Discourse forum](https://discourse.charmhub.io/t/welcome-to-the-charmed-operator-community).