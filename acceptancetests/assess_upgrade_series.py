#!/usr/bin/env python
"""Assess upgrading using the 'upgrade-series' commands."""

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
from fixtures import EnvironmentVariable
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

__metaclass__ = type

log = logging.getLogger("assess_upgrade_series")
charm_bundle = 'upgrade-series.yaml'


def assess_juju_upgrade_series(client, args):
    target_machine = '0'
    upgrade_series_prepare(client, target_machine, args.to_series, True)
    do_release_upgrade(client, target_machine)
    reboot_machine(client, target_machine)
    upgrade_series_complete(client, target_machine)
    assert_correct_series(client, target_machine, args.to_series, True)


def upgrade_series_prepare(client, machine, series, agree=False):
    args = (machine, series)
    if agree:
        args += ('--agree',)
    client.juju('upgrade-series prepare', args)


def upgrade_series_complete(client, machine):
    args = (machine)
    client.juju('upgrade-series complete', args)


def do_release_upgrade(client, machine):
    try:
        output = client.get_juju_output(
            'ssh', machine, 'sudo do-release-upgrade -f '
            'DistUpgradeViewNonInteractive', timeout=3600)
    except subprocess.CalledProcessError as e:
        raise AssertionError(
            "do-release-upgrade failed on {}: {}".format(machine, e))

    log.info("do-release-upgrade response: ".format(output))


def reboot_machine(client, machine):
    try:
        log.info("Restarting: {}".format(machine))
        cmd = build_ssh_cmd(client, machine, 'sudo shutdown -r now && exit')
        output = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
        log.info("Restarting machine output: {}\n".format(output))
    except subprocess.CalledProcessError as e:
        logging.info(
            "Error running shutdown:\nstdout: %s\nstderr: %s",
            e.output, getattr(e, 'stderr', None))

    log.info("wait_for_started()")
    client.wait_for_started()


def build_ssh_cmd(client, machine, command):
    ssh_opts = [
        "-o", "User ubuntu",
        "-o", "UserKnownHostsFile /dev/null",
        "-o", "StrictHostKeyChecking no",
        "-o", "PasswordAuthentication no",
    ]

    status = client.get_status()
    machine_status = status.get_machine(machine)
    cmd = ["ssh"] + ssh_opts + [machine_status['public-address']] + [command]
    return cmd


def assert_correct_series(client, machine, expected):
    """Verify that juju knows the correct series for the machine"""
    status = client.get_status()
    status = status.get_machine(machine)
    machine_series = status['series']
    if machine_series is not expected:
        raise JujuAssertionError(
            "Machine {} series doesn't match the expected series: {}"
            .format(machine, expected))


def setup(client, start_series):
    """ Deploy charms, there are several under ./repository """
    charm_source = local_charm_path(
        charm=charm_bundle,
        repository=os.environ['JUJU_REPOSITORY'],
        juju_ver=client.version)
    _, deploy_complete = client.deploy(charm_source)
    log.info("Deployed {} with {}".format(charm_bundle, start_series))
    # Wait for the deployment to finish.
    client.wait_for(deploy_complete)


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju update series.")
    add_basic_testing_arguments(parser)
    parser.add_argument('--from-series', default='xenial', dest='from_series',
                        help='Series to start machine and units with')
    parser.add_argument('--to-series', default='bionic', dest='to_series',
                        help='Series to upgrade machine and units to')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        setup(bs_manager.client, args.from_series)
        with EnvironmentVariable('JUJU_DEV_FEATURE_FLAGS', 'upgrade-series'):
            assess_juju_upgrade_series(bs_manager.client, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
