#!/usr/bin/env python
"""This module tests the deployment with constraints."""

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

VIRT_TYPES = ['lxd']


def form_constraint(constraint_name, constraint_value):
    """Form a constraint and return a tuple containing what is used."""
    if constraint_value is not None:
        return (constraint_name + '={}'.format(constraint_value),)
    else:
        return ()


def make_constraints(memory=None, cpu_cores=None, virt_type=None):
    """Construct a contraints argument string from contraint values."""
    args = ()
    args += form_constraint('mem', memory)
    args += form_constraint('cpu-cores', cpu_cores)
    args += form_constraint('virt-type', virt_type)
    return ' '.join(args)


def deploy_constraint(client, charm, series, charm_repo, constraints):
    """Test deploying charm with constraints."""
    client.deploy(charm, series=series, repository=charm_repo,
                  constraints=constraints)
    client.wait_for_workloads()


def deploy_charm_constraint(client, charm_name, charm_series, constraints):
    """Create a charm with constraints and test deploying it."""
    with temp_dir() as charm_dir:
        constraints_charm = Charm(charm_name,
                                  'Test charm for constraints',
                                  series=['xenial'])
        charm_root = constraints_charm.to_repo_dir(charm_dir)
        platform = 'ubuntu'
        charm = local_charm_path(charm=charm_name,
                                 juju_ver=client.version,
                                 series=charm_series,
                                 repository=os.path.dirname(charm_root),
                                 platform=platform)
        deploy_constraint(client, charm, charm_series,
                          charm_dir, constraints)


def assess_virt_type(client, virt_type):
    """Assess the virt-type option for constraints"""
    if virt_type not in VIRT_TYPES:
        raise JujuAssertionError(virt_type)
    constraints = make_constraints(virt_type=virt_type)
    charm_name = 'virt-type-' + virt_type
    charm_series = 'xenial'
    deploy_charm_constraint(client, charm_name, charm_series, constraints)


def assess_virt_type_constraints(client, test_kvm=False):
    """Assess deployment with virt-type constraints."""
    if test_kvm:
        VIRT_TYPES.append("kvm")
    for virt_type in VIRT_TYPES:
        assess_virt_type(client, virt_type)
    try:
        assess_virt_type(client, 'aws')
    except JujuAssertionError:
        log.info("Correctly rejected virt-type aws")
    else:
        raise JujuAssertionError("FAIL: Client deployed with virt-type aws")
    if test_kvm:
        VIRT_TYPES.remove("kvm")


def assess_constraints(client, test_kvm=False):
    """Assess deployment with constraints."""
    assess_virt_type_constraints(client, test_kvm)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test constraints")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    test_kvm = '--with-virttype-kvm' in args
    with bs_manager.booted_context(args.upload_tools):
        assess_constraints(bs_manager.client, test_kvm)
    return 0


if __name__ == '__main__':
    sys.exit(main())
