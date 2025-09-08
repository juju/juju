(client)=
# Client

A Juju **client** is any software that implements th Juju client apiserver contract and is able to talk to a Juju {ref}`controller <controller>`.

This currently includes:

- {ref}`the juju CLI <juju-cli>`
- JIMM, the central management component of [JAAS](https://documentation.ubuntu.com/jaas/)
- [Jubilant](https://documentation.ubuntu.com/jubilant/)
- the [Terraform Provider for Juju](https://documentation.ubuntu.com/terraform-provider-juju/latest/)
- [Python Libjuju](https://pythonlibjuju.readthedocs.io/en/latest/) (legacy; please use [Jubilant](https://documentation.ubuntu.com/jubilant/))

Note: While the various clients generally aim for feature parity, there are still differences between them coming from the nature of the client (e.g., interactive vs. declarative vs. programmatic). Also, the only client that can currently bootstrap a Juju cotroller is the `juju` CLI client.