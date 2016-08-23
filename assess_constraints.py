#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""

from __future__ import print_function
import os

import argparse
import logging
import sys

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import (
    Charm,
    local_charm_path,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_constraints")

VIRT_TYPES = ['kvm', 'lxd']


def deploy_constraint(client, charm, series, charm_repo,
                      memory=None, cpu_cores=None, virt_type=None):
    """Test deploying charm with constraints."""
    args = ()
    if memory:
        args += ("mem={}".format(memory),)
    if cpu_cores:
        args += ("cpu-cores={}".format(cpu_cores),)
    if virt_type:
        args += ("virt-type={}".format(virt_type),)
    constraints = " ".join(args)
    client.deploy(charm, series=series, repository=charm_repo,
                  constraints=constraints)
    client.wait_for_started()


def assess_virt_type(client, virt_type):
    """Assess the virt-type option for constraints"""
    if virt_type not in VIRT_TYPES:
        raise JujuAssertionError(virt_type)
    charm_name = 'virt-type-' + virt_type
    charm_series = 'trusty'
    with temp_dir() as charm_dir:
        constraints_charm = Charm(charm_name,
                                  'Test charm for constraints',
                                  series=['trusty'])
        charm_root = constraints_charm.to_repo_dir(charm_dir)
        platform = 'ubuntu'
        charm = local_charm_path(charm=charm_name,
                                 juju_ver=client.version,
                                 series=charm_series,
                                 repository=os.path.dirname(charm_root),
                                 platform=platform)
        deploy_constraint(client, charm, charm_series,
                          charm_dir, virt_type=virt_type)


def assess_constraints(client):
    """Assess deployment with constraints"""
    for virt_type in VIRT_TYPES:
        assess_virt_type(client, virt_type)
    try:
        assess_virt_type(client, 'aws')
    except JujuAssertionError:
        log.info("Correctly rejected virt-type aws")
    else:
        raise JujuAssertionError("FAIL: Client deployed with virt-type aws")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test constraints")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_constraints(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
