#!/usr/bin/env python
"""This module tests the deployment with constraints."""
# Currently in a major re-work.

from __future__ import print_function
import os

import argparse
import logging
import sys

import yaml

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

INSTANCE_TYPES = {
    'azure': [],
    'ec2': [],
    'gce': [],
    'joyent': [],
    'openstack': [],
    }


def append_constraint(list, constraint_name, constraint_value):
    """Append a constraint to a list of constraints if it is used."""
    if constraint_value is not None:
        list.append('{}={}'.format(constraint_name, constraint_value))


def make_constraints(memory=None, cpu_cores=None, virt_type=None,
                     instance_type=None, root_disk=None, cpu_power=None):
    """Construct a contraints argument string from contraint values."""
    args = []
    append_constraint(args, 'mem', memory)
    append_constraint(args, 'cpu-cores', cpu_cores)
    append_constraint(args, 'virt-type', virt_type)
    append_constraint(args, 'instance-type', instance_type)
    append_constraint(args, 'root-disk', root_disk)
    append_constraint(args, 'cpu-power', cpu_power)
    return ' '.join(args)


def deploy_constraint(client, constraints, charm, series, charm_repo):
    """Test deploying charm with constraints."""
    client.deploy(charm, series=series, repository=charm_repo,
                  constraints=constraints)
    client.wait_for_workloads()


def deploy_charm_constraint(client, constraints, charm_name, charm_series,
                            charm_dir):
    """Create a charm with constraints and test deploying it."""
    constraints_charm = Charm(charm_name,
                              'Test charm for constraints',
                              series=[charm_series])
    charm_root = constraints_charm.to_repo_dir(charm_dir)
    platform = 'ubuntu'
    charm = local_charm_path(charm=charm_name,
                             juju_ver=client.version,
                             series=charm_series,
                             repository=os.path.dirname(charm_root),
                             platform=platform)
    deploy_constraint(client, constraints, charm,
                      charm_series, charm_dir)


def assess_virt_type(client, virt_type):
    """Assess the virt-type option for constraints"""
    if virt_type not in VIRT_TYPES:
        raise JujuAssertionError(virt_type)
    constraints = make_constraints(virt_type=virt_type)
    charm_name = 'virt-type-' + virt_type
    charm_series = 'xenial'
    with temp_dir() as charm_dir:
        deploy_charm_constraint(client, constraints, charm_name,
                                charm_series, charm_dir)


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


def assess_instance_type(client, provider, instance_type):
    """Assess the instance-type option for constraints"""
    if instance_type not in INSTANCE_TYPES[provider]:
        raise JujuAssertionError(instance_type)
    constraints = make_constraints(instance_type=instance_type)
    charm_name = 'instance-type-' + instance_type
    charm_series = 'xenial'
    with temp_dir() as charm_dir:
        deploy_charm_constraint(client, constraints, charm_name,
                                charm_series, charm_dir)


def assess_instance_type_constraints(client):
    """Assess deployment with instance-type constraints."""
    provider = client.env.config.get('type')
    if provider not in INSTANCE_TYPES:
        raise ValueError('Provider does not implement instance-type '
                         'constraint.')
    for instance_type in INSTANCE_TYPES[provider]:
        assess_instance_type(client, provider, instance_type)


def juju_show_machine_hardware(client, machine):
    """Uses juju show-machine and returns information about the hardwere."""
    raw = client.get_juju_output('show-machine', machine, '--format', 'yaml')
    raw_yaml = yaml.load(raw)
    hardware = raw_yaml['machines'][machine]['hardware']
    data = {}
    for kvp in hardware.split(' '):
        (key, value) = kvp.split('=')
        data[key] = value
    return data


def assess_constraints_lxd(client, args):
    """Run the tests that are used on lxd."""
    charm_series = (args.series or 'xenial')
    with temp_dir() as charm_dir:
        charm_name = 'lxd-root-disk-2048'
        constraints = make_constraints(root_disk='2048')
        deploy_charm_constraint(client, constraints, charm_name,
                                charm_series, charm_dir)
        # Check the machine for the amount of disk-space.
        client.remove_service(charm_name)


def assess_constraints_ec2(client):
    """Run the tests that are used on ec2."""
    charm_series = 'xenial'
    with temp_dir() as charm_dir:
        charm_name = 'ec2-instance-type-t2.small'
        constraints = make_constraints(instance_type='t2.small')
        deploy_charm_constraint(client, constraints, charm_name,
                                charm_series, charm_dir)
        # Check the povider for the instance-type
        client.remove_service(charm_name)


def assess_constraints(client, test_kvm=False):
    """Assess deployment with constraints."""
    assess_virt_type_constraints(client, test_kvm)
    assess_instance_type_constraints(client)


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
        #assess_constraints_lxd(bs_manager.client, args)
        assess_constraints(bs_manager.client, test_kvm)
    return 0


if __name__ == '__main__':
    sys.exit(main())
