#!/usr/bin/env python
"""This module tests the deployment with constraints."""
# Currently in a major re-work.
# ./assess_constraints.py parallel-lxd /usr/lib/juju-2.0/bin/juju
# Errors out, because of a bad charm path. I have chased it all the way to
# backend.juju, the correct argument is still being passed there.

from __future__ import print_function
import os
import argparse
import logging
import sys
import re
from contextlib import contextmanager

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
        # t2.micro hardware: arch=amd64 cpu-cores=1 cpu-power=10
        #                    mem=1024M root-disk=8192M
        't2.micro': {'root_disk': '1G', 'cpu_power': '10', 'cores': '1'},
        't2.large': {'root_disk': '8G', 'cpu_power': '20', 'cores': '1'},
        }[instance_type]


def mem_as_int(size):
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
        return (self.mem == other.mem and self.cores == other.cores
                and self.virt_type == other.virt_type
                and self.instance_type == other.instance_type
                and self.root_disk == other.root_disk
                and self.cpu_power == other.cpu_power
                and self.arch == other.arch
                )

    def meets_root_disk(self, actual_root_disk):
        """Check to see if a given value meets the root_disk constraint."""
        if self.root_disk is None:
            return True
        return mem_as_int(self.root_disk) <= mem_as_int(actual_root_disk)

    def meets_cores(self, actual_cores):
        """Check to see if a given value meets the cores constraint."""
        if self.cores is None:
            return True
        return int(self.cores) <= int(actual_cores)

    def meets_cpu_power(self, actual_cpu_power):
        """Check to see if a given value meets the cpu_power constraint."""
        if self.cpu_power is None:
            return True
        return int(self.cpu_power) <= int(actual_cpu_power)

    def meets_arch(self, actual_arch):
        """Check to see if a given value meets the arch constraint."""
        if self.arch is None:
            return True
        return self.arch == actual_arch

    def meets_instance_type(self, actual_data):
        """Check to see if a given value meets the instance_type constraint.

        Currently there is no direct way to check for it, so we 'fingerprint'
        each instance_type in a dictionary."""
        instance_data = get_instance_spec(self.instance_type)
        for (key, value) in instance_data.iteritems():
            if key not in actual_data:
                raise JujuAssertionError('Missing data:', key)
            elif value != actual_data[key]:
                return False
        else:
            return True


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


@contextmanager
def deploy_context(client, constraint, charm_name, charm_series, charm_dir):
    """Deploy a charm and then take it back down."""
    deploy_charm_constraint(client, constraint, charm_name,
                            charm_series, charm_dir)
    yield
    client.remove_service(charm_name)


def assess_constraints_lxd(client, args):
    """Run the tests that are used on lxd."""
    charm_series = (args.series or 'xenial')
    with temp_dir() as charm_dir:
        charm_name = 'lxd-root-disk-2048'
        constraints = Constraints(root_disk='2048')
        with deploy_context(client, str(constraints), charm_name,
                            charm_series, charm_dir):
            data = juju_show_machine_hardware(client, 0)
            if not constraints.meets_root_disk(data['root-disk']):
                JujuAssertionError('Not enough space on the root disk.')


def assess_constraints_ec2(client):
    """Run the tests that are used on ec2."""
    charm_series = 'xenial'
    with temp_dir() as charm_dir:
        charm_name = 'ec2-instance-type-t2.micro'
        constraints = Constraints(instance_type='t2.micro')
        with deploy_context(client, str(constraints), charm_name,
                            charm_series, charm_dir):
            data = juju_show_machine_hardware(client, '0')
            if not constraints.meets_instance_type(data):
                JujuAssertionError('Instance type did not match.')
        charm_name = 'ec2-cores-2'
        constraints = Constraints(cores='2')
        with deploy_context(client, str(constraints), charm_name,
                            charm_series, charm_dir):
            data = juju_show_machine_hardware(client, '0')
            if not constraints.meets_cores(data['cores']):
                JujuAssertionError('Not enough cores on machine.')
        charm_name = 'ec2-cpu-power-30'
        constraints = Constraints(cpu_power='30')
        with deploy_context(client, str(constraints), charm_name,
                            charm_series, charm_dir):
            data = juju_show_machine_hardware(client, '0')
            if not constraints.meets_cpu_power(data['cpu_power']):
                JujuAssertionError('CPU does not have enough power.')


def assess_constraints(client, test_kvm=False):
    """Assess deployment with constraints."""
    assess_virt_type_constraints(client, test_kvm)
    #assess_instance_type_constraints(client)


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
