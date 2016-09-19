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

INSTANCE_TYPES = {
    'azure': [],
    'ec2': ['t2.micro'],
    'gce': [],
    'joyent': [],
    'openstack': [],
    }


# This assumes instances are unique accross providers.
def get_instance_spec(instance_type):
    """Get the specifications of a given instance type."""
    return {
        # t2.micro hardware: arch=amd64 cpu-cores=1 cpu-power=10
        #                    mem=1024M root-disk=8192M
        't2.micro': {'root_disk': '1G', 'cpu_power': '10', 'cores': '1'},
        't2.large': {'root_disk': '8G', 'cpu_power': '20', 'cores': '1'},
        }[instance_type]


class Constraints:
    """Class that repersents a set of contraints."""

    @staticmethod
    def _list_to_str(constraints_list):
        parts = []
        for (name, value) in constraints_list:
            if value is not None:
                parts.append('{}={}'.format(name, value))
        return ' '.join(parts)

    @staticmethod
    def str(mem=None, cores=None, virt_type=None, instance_type=None,
            root_disk=None, cpu_power=None, arch=None):
        """Convert the given constraint values into an argument string."""
        return Constraints._list_to_str(
            [('mem', mem), ('cores', cores), ('virt-type', virt_type),
             ('instance-type', instance_type), ('root-disk', root_disk),
             ('cpu-power', cpu_power), ('arch', arch),
             ])

    def __init__(self, mem=None, cores=None, virt_type=None,
                 instance_type=None, root_disk=None, cpu_power=None,
                 arch=None):
        """Create a new constraints instance from individual constraints."""
        self.mem = mem
        self.cores = cores
        self.virt_type = virt_type
        self.instance_type = instance_type
        self.root_disk = root_disk
        self.cpu_power = cpu_power
        self.arch = arch

    def __str__(self):
        """Convert the instance constraint values into an argument string."""
        return Constraints.str(
            self.mem, self.cores, self.virt_type, self.instance_type,
            self.root_disk, self.cpu_power, self.arch
            )

    def __eq__(self, other):
        return (self.mem == other.mem and self.cores == other.cores and
                self.virt_type == other.virt_type and
                self.instance_type == other.instance_type and
                self.root_disk == other.root_disk and
                self.cpu_power == other.cpu_power and
                self.arch == other.arch
                )


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
    constraints = Constraints.str(virt_type=virt_type)
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
    constraints = Constraints.str(instance_type=instance_type)
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
