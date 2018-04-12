# Overview

This charm provisions the Juju CI testing tools needed to run tests on a
jenkins slave.


# Usage

To deploy a Jenkins slave you will also need to deploy the jenkins master
charm. This can be done as follows:

    juju deploy jenkins
    juju deploy jenkins-slave
    juju deploy jenkins-juju-ci
    juju add-relation jenkins jenkins-slave
    juju add-relation jenkins-juju-ci jenkins-slave
