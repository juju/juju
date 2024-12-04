(ops)=
# Ops (`ops`)

In the charm SDK, the Ops (ops) library is a Python framework (available on PyPI) for developing and testing Juju charms with standard Python constructs, allowing for clean, maintainable, and reusable code and making charm development simple.

Ops provides an object-oriented, high-level wrapper to respond to Juju hooks and interact with hook tools, among other things. 

While charm development has gone through multiple stages, Ops is the current state of the art. 

> See more: [About charming history](https://juju.is/docs/sdk/history)

The remainder of this doc gives a high-level overview, highlighting the main abstractions. 

> For more, read the [Ops library API reference](https://ops.readthedocs.io/en/latest/), and see the source code on [GitHub](https://github.com/canonical/operator). 

