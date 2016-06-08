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

## Requirements for running tests
You will need some python libraries in order to run the existing CI tests.
Install the following

sudo apt-get install python-launchpadlib python-yaml python-boto python-mock
 python-jenkins python-novaclient python-pexpect python-winrm python-coverage

In addition, if you wish to use azure, you will need to install pip, and the
associated client library from pip for azure.

sudo apt-get install python-pip
pip install azure

# Creating a New CI Test

Test scripts will be run under many conditions to reproduce real cases.
Most scripts cannot assume special knowledge of the substrate, region,
bootstrap constraints, tear down, and log collection, etc.

If this is your first time, consider asking one of the QA team to pair-program
on it with you.

You can base your new script and its unit tests on the template files.
They provide the infrastructure to setup and tear down a test. Your script
can focus on the unique aspects of your test. Start by making a copy of
template_assess.py.tmpl, and don't forget unit tests!

    make new-assess name=my_function

Run make lint early and often. (You may need to do sudo apt-get install python-
flake8). If you forget, you can run autopep8 to fix certain issues. Please use
--ignore E24,E226,E123 with autopep8. Code that's been hand-written to follow
PEP8 is generally more readable than code which has been automatically
reformatted after the fact. By running make lint often, you'll absorb the style
and write nice PEP8-compliant code.

Please avoid creating diffs longer than 400 lines. If you are writing a new
test, that may mean creating it as a series of branches. You may find bzr-
pipeline to be a useful tool for managing a series of branches.

If your tests require new charms, please write them in Python.
