---
myst:
  html_meta:
    description: "Complete lifecycle guide for Juju deployments: set up, harden, troubleshoot, upgrade, and tear down cloud deployments."
---

(manage-your-deployment)=
# How to manage your Juju deployment

The goal of everything in Juju is to help you set up and maintain your cloud deployment, from day 0 to day 2, in the same unified way, on any cloud and even between clouds. This series of documents covers the high-level logic.

```{toctree}
:titlesonly:
:glob:
:hidden:

manage-your-juju-deployment/set-up-your-juju-deployment
manage-your-juju-deployment/set-up-your-juju-deployment-local-testing-and-development
manage-your-juju-deployment/set-up-your-juju-deployment-offline
manage-your-juju-deployment/harden-your-juju-deployment
manage-your-juju-deployment/troubleshoot-your-juju-deployment
manage-your-juju-deployment/upgrade-your-juju-deployment
manage-your-juju-deployment/upgrade-juju-36-to-40
manage-your-juju-deployment/tear-down-your-juju-deployment-local-testing-and-development

```

First you'll want to set things up:

- {ref}`set-up-your-deployment`

The specifics may be simpler if you're working locally (e.g., for local testing and development):

- {ref}`set-things-up`

Or more sophisticated, if you're working in a proxy-restricted environment:

- {ref}`take-your-deployment-offline`

Whichever the case, make sure to harden:

- {ref}`harden-your-deployment`

If things don't go as expected, here's how to troubleshoot:

- {ref}`troubleshoot-your-deployment`

At all time, try to stay up to date:

- {ref}`upgrade-your-deployment`

If you're upgrading from Juju 3.6 to 4.0, check out the specific guide for that:

- {ref}`upgrade-your-deployment-from-36-to-40`

And, if you're trying things locally, here's how to clean up:

- {ref}`tear-things-down`