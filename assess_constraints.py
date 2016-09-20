#!/usr/bin/env python
"""This module tests the deployment with constraints."""

from __future__ import print_function
import argparse
import logging
import os
import sys
import re

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
    'ec2': ['t2.micro'],
    'gce': [],
    'joyent': [],
    'openstack': [],
    }


# This assumes instances are unique accross providers.
def get_instance_spec(instance_type):
    """Get the specifications of a given instance type."""
    return {
        't2.micro': {'mem': '1G', 'cpu-power': '10', 'cores': '1'},
        }[instance_type]


def mem_to_int(size):
    """Convert an argument size into a number of megabytes."""
    if not re.match(re.compile('^[0123456789]+[MGTP]?$'), size):
        raise JujuAssertionError('Not a size format:', size)
    if size[-1] in 'MGTP':
        val = int(size[0:-1])
        unit = size[-1]
        return val * (1024 ** 'MGTP'.find(unit))
    else:
        return int(size)


class Constraints:
    """Class that represents a set of contraints."""

    @staticmethod
    def _list_to_str(constraints_list):
        parts = ['{}={}'.format(name, value) for (name, value) in
                 constraints_list if value is not None]
        return ' '.join(parts)

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
        return Constraints._list_to_str(
            [('mem', self.mem), ('cores', self.cores),
             ('virt-type', self.virt_type),
             ('instance-type', self.instance_type),
             ('root-disk', self.root_disk), ('cpu-power', self.cpu_power),
             ('arch', self.arch),
             ])

    def __eq__(self, other):
        return (self.mem == other.mem and self.cores == other.cores and
                self.virt_type == other.virt_type and
                self.instance_type == other.instance_type and
                self.root_disk == other.root_disk and
                self.cpu_power == other.cpu_power and
                self.arch == other.arch
                )

    @staticmethod
    def _meets_string(constraint, actual):
        if constraint is None:
            return True
        return constraint == actual

    @staticmethod
    def _meets_min_int(constraint, actual):
        if constraint is None:
            return True
        return int(constraint) <= int(actual)

    @staticmethod
    def _meets_min_mem(constraint, actual):
        if constraint is None:
            return True
        return mem_to_int(constraint) <= mem_to_int(actual)

    def meets_root_disk(self, actual_root_disk):
        """Check to see if a given value meets the root_disk constraint."""
        return self._meets_min_mem(self.root_disk, actual_root_disk)

    def meets_cores(self, actual_cores):
        """Check to see if a given value meets the cores constraint."""
        return self._meets_min_int(self.cores, actual_cores)

    def meets_cpu_power(self, actual_cpu_power):
        """Check to see if a given value meets the cpu_power constraint."""
        return self._meets_min_int(self.cpu_power, actual_cpu_power)

    def meets_arch(self, actual_arch):
        """Check to see if a given value meets the arch constraint."""
        return self._meets_string(self.arch, actual_arch)

    def meets_instance_type(self, actual_data):
        """Check to see if a given value meets the instance_type constraint.

        Currently there is no direct way to check for it, so we 'fingerprint'
        each instance_type in a dictionary."""
        instance_data = get_instance_spec(self.instance_type)
        for (key, value) in instance_data.iteritems():
            # Temperary fix until cpu-cores -> cores switch is finished.
            if key is 'cores' and 'cpu-cores' in actual_data:
                key = 'cpu-cores'
            if key not in actual_data:
                raise JujuAssertionError('Missing data:', key)
            elif key in ['mem', 'root-disk']:
                if mem_to_int(value) != mem_to_int(actual_data[key]):
                    return False
            elif value != actual_data[key]:
                return False
        else:
            return True


def deploy_constraint(client, constraints, charm, series, charm_repo):
    """Test deploying charm with constraints."""
    client.deploy(charm, series=series, repository=charm_repo,
                  constraints=str(constraints))
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


def juju_show_machine_hardware(client, machine):
    """Uses juju show-machine and returns information about the hardware."""
    raw = client.get_juju_output('show-machine', machine, '--format', 'yaml')
    raw_yaml = yaml.load(raw)
    try:
        hardware = raw_yaml['machines'][machine]['hardware']
    except KeyError as error:
        raise KeyError(error.args, raw_yaml)
    data = {}
    for kvp in hardware.split(' '):
        (key, value) = kvp.split('=')
        data[key] = value
    return data


def prepare_constraint_test(client, constraints, charm_name,
                            charm_series='xenial', machine='0'):
    """Deploy a charm with constraints and data to see if it meets them."""
    with temp_dir() as charm_dir:
        deploy_charm_constraint(client, constraints, charm_name,
                                charm_series, charm_dir)
        client.wait_for_started()
        return juju_show_machine_hardware(client, machine)


def assess_virt_type(client, virt_type):
    """Assess the virt-type option for constraints"""
    if virt_type not in VIRT_TYPES:
        raise JujuAssertionError(virt_type)
    constraints = Constraints(virt_type=virt_type)
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
    constraints = Constraints(instance_type=instance_type)
    charm_name = 'instance-type-' + instance_type.replace('.', '-')
    data = prepare_constraint_test(client, constraints, charm_name)
    if not constraints.meets_instance_type(data):
        raise JujuAssertionError('Test failed', charm_name)


def assess_instance_type_constraints(client, provider=None):
    """Assess deployment with instance-type constraints."""
    if provider is None:
        provider = client.env.config.get('type')
    if provider not in INSTANCE_TYPES:
        return
    for instance_type in INSTANCE_TYPES[provider]:
        assess_instance_type(client, provider, instance_type)


def assess_root_disk_constraints(client, values):
    """Assess deployment with root-disk constraints."""
    for root_disk in values:
        constraints = Constraints(root_disk=root_disk)
        charm_name = 'root-disk-' + root_disk
        data = prepare_constraint_test(client, constraints, charm_name)
        if not constraints.meets_root_disk(data['root-disk']):
            raise JujuAssertionError('Test failed', charm_name)


def assess_cores_constraints(client, values):
    """Assess deployment with cores constraints."""
    for cores in values:
        constraints = Constraints(cores=cores)
        charm_name = 'cores-' + cores
        data = prepare_constraint_test(client, constraints, charm_name)
        if not constraints.meets_cores(data['cores']):
            raise JujuAssertionError('Test failed', charm_name)


def assess_cpu_power_constraints(client, values):
    """Assess deployment with cpu_power constraints."""
    for cpu_power in values:
        constraints = Constraints(cpu_power=cpu_power)
        charm_name = 'cpu_power-' + cpu_power
        data = prepare_constraint_test(client, constraints, charm_name)
        if not constraints.meets_root_disk(data['cpu_power']):
            raise JujuAssertionError('Test failed', charm_name)


def assess_constraints(client, test_kvm=False):
    """Assess deployment with constraints."""
    provider = client.env.config.get('type')
    if 'lxd' == provider:
        assess_virt_type_constraints(client, test_kvm)
    elif 'ec2' == provider:
        assess_instance_type_constraints(client, provider)


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
