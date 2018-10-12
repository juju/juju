#!/usr/bin/env python
"""Assess when deploying with LXD Profile charm using the 'deploy' command."""

from __future__ import print_function

import argparse
import logging
import os
import subprocess
import sys

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import (
    local_charm_path,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

__metaclass__ = type

log = logging.getLogger("assess_lxdprofile_deploy_charm")
charm_bundle = 'lxd-profile.yaml'

def setup(client, series):
    charm_sink = local_charm_path(
        charm='charms/lxd-profile',
        juju_ver=client.version,
        series=series,
        repository=os.environ['JUJU_REPOSITORY'])
    _, primary = client.deploy(charm_sink, series=series)
    client.wait_for(primary)

def assess_juju_lxdprofile_machine(client, args):
    """ Tests juju status

    Verify with juju status
    """
    lxdprofile_machine_verify(client, "juju-{}-lxd-profile-0".format(client.model_name))

def lxdprofile_machine_verify(client, profilename):
    """ Checks the status output is the same as profilename

    :param client: Juju client
    :param profilename: LXD Profile name to expect in the output
    :return: None
    :raises JujuAssertionError: if profilename is not appropriately shown.
    """
    status = client.get_status()
    machine_info = dict(status.iter_machines())

    machine_lxdprofile = machine_info["0"]["lxd-profiles"]
    log.info(
        "profile name {}, machine lxd profile {}".format(profilename, machine_lxdprofile))

    if profilename not in machine_lxdprofile:
        raise JujuAssertionError(
            "LXD profile in juju status for machine-0 is not {}, per juju".format(
                profilename))

    log.info("juju status 0 {} succeeded".format(profilename))

def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju lxd profile deploy.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        setup(bs_manager.client, args.series)
        assess_juju_lxdprofile_machine(bs_manager.client, args)
    return 0

if __name__ == '__main__':
    sys.exit(main())
