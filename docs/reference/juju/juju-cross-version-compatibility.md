(juju-cross-version-compatibility)=
# Juju component cross-version compatibility

Commonly, you may have to work with multiple versions of Juju at once. This document describes the compatibility rules between different versions of Juju.

## {ref}`juju-cli`, {ref}`controllers <controller>`, and {ref}`agents <agent>

Juju controllers, agents, and the `juju` CLI client all are [semantically versioned](https://semver.org/). This means:
- Controllers/agents/clients **in the same major/minor series** (e.g. 3.5.0 and 3.5.2) are fully compatible.
- Controllers/agents/clients **in the same major series** (e.g. 3.4 and 3.5) are compatible, but older versions may be lacking features present in newer versions.
- Controllers/agents/clients with different major versions (e.g. 2.8 and 3.1) are **not guaranteed to be compatible.** The one exception is that we guarantee a basic set of operations (e.g. status and migration) is compatible between **the last minor in a major series** and the next major. This enables users to upgrade their existing deployments to the next major version. Related to that: A Juju client only bootstraps a Juju controller of the same major/minor version.

<!--
**Patch versions are fully compatible. Minor versions are compatible, modulo new features. (That's just the usual semantic versioning rules.) A major version client is compatible with the last minor of the previous major, modulo new features. Caveat: A Juju client only bootstraps a Juju controller of the same major/minor version.**
-->

## [python-libjuju`](https://pythonlibjuju.readthedocs.io/en/latest/)

- For a 2.9.x controller, you should use the latest python-libjuju in the 2.9 track.
- For 3.x controllers, you should use the latest version of python-libjuju.

## [terraform-provider-juju](https://canonical-terraform-provider-juju.readthedocs-hosted.com/en/latest/tutorial/)

The latest version of the Terraform Juju provider should be compatible with all Juju controllers.
