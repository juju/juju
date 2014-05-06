# Continuous Integration scripts

The Juju QA team uses a common set or tests that exercise the release tools
and juju to verify that each revision of juju can be released, built, packaged,
published, installed, bootstrapped, and deployed.

CI requires
    lp:juju-release-tools (The packaging and publication tools)
    lp:juju-ci-tools (This branch)
    lp:juju-ci-tools/repository (A copy of the juju charm repository)
    And a JUJU_HOME with all the envs to test.

The general process involves making a release tarball, making a package,
making a tree of tools and metadata, and lastly publishing the tools.
you can skip the tarball and package steps if you just want to publish
the juju tools (AKA jujud, servers, agents). If you want to test a fix
that is in the juju-core trunk branch, you can make your own release
tarball and package.

Once juju is published to the test clouds, individual tests can be performed.
Most tests accept an env name. The envs define the cloud and series to test.

