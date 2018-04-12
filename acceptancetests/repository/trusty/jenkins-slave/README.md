# Overview

This charm provisions a Jenkins slave to connect to a Jenkins master.
This is the companion to the Jenkins charm.


# Usage

To deploy a Jenkins slave you will also need to deploy the jenkins master
charm. This can be done as follows:

    juju deploy jenkins
    juju deploy -n 5 jenkins-slave
    juju add-relation jenkins jenkins-slave

There are cases where you want to provision a specific machine that
provides specific resources for tests, such as CPU architecture or
network access. You can deploy the extra slave like this:

    juju add-machine <special-machine-private-ip>
    juju deploy --to <special-mabine-number> jenkins-slave ppc-slave

See the Jenkins charm for more details.
