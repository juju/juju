JujuPy
######

A library for driving the Juju commandline client, created by the Juju team.

This library is compatible with released juju versions in the 1.x and 2.x
series, beginning with 1.18.  It has been used in production since 2013, but
was extracted into a separate library in 2017.

It provides specific support for many commands.  In cases where it does not
provide specific support, the normal juju commandline can be used through the
``juju`` and ``get_juju_output`` methods.

It provides ways of checking health, as well as waiting for an operation to
complete.

It supports interactive commands like ``register``.

It is well-tested, with more than 8K lines of tests against more than 5K lines
of code.

It provides fakes for testing, i.e. a fake backend and the convenience
function ``get_fake_client``.
