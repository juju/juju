#!/usr/bin/env python3
"""Assess upgrading using the 'upgrade-series' commands."""

from __future__ import print_function

import argparse
import logging
import os
import subprocess
import sys
import time

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

log = logging.getLogger("assess_upgrade_series")


DEFAULT_FROM_SERIES = 'xenial'
DEFAULT_TO_SERIES = 'bionic'


def assess_juju_upgrade_series(client, args):
    target_machine = '0'
    assert_correct_series(client, target_machine, args.from_series)
    upgrade_series_prepare(client, target_machine, args.to_series, agree=True)
    reboot_machine(client, target_machine)
    do_release_upgrade(client, target_machine)
    reboot_machine(client, target_machine)
    upgrade_series_complete(client, target_machine)
    log.info("waiting for the machine agent to report the new series")
    time.sleep(5)
    assert_correct_series(client, target_machine, args.to_series)


def upgrade_series_prepare(client, machine, series, **flags):
    args = (machine, 'prepare', series)
    if flags['agree']:
        args += ('-y',)
    client.juju('upgrade-series', args)


def upgrade_series_complete(client, machine):
    args = (machine, 'complete')
    client.juju('upgrade-series', args)


def do_release_upgrade(client, machine):
    try:
        output = client.get_juju_output(
            'ssh', machine, 'sudo do-release-upgrade -f '
            'DistUpgradeViewNonInteractive', timeout=3600)
        log.info("do-release-upgrade response: {}".format(output))
    except subprocess.CalledProcessError as e:
        raise AssertionError(
            "do-release-upgrade failed on {}: {}".format(machine, e))


def reboot_machine(client, machine):
    """Issue a reboot command to the machine via `juju ssh`.
    The issued command may exit with a 255 status to indicate that the remote
    is not available. We ignore this.
    """
    log.info("Restarting: {}".format(machine))

    try:
        client.juju('ssh', (machine, 'sudo shutdown -r now'))
    except subprocess.CalledProcessError as e:
        if e.returncode != 255:
            raise e
        log.info("Ignoring `juju ssh` exit status after triggering reboot")

    log.info("wait_for_started()")
    client.wait_for_started()


def assert_correct_series(client, machine, expected):
    """Verify that juju knows the correct series for the machine"""
    status = client.get_status()
    machine_series = status.status['machines'][machine]['series']
    if machine_series != expected:
        raise JujuAssertionError(
            "Machine {} series of {} doesn't match the expected series: {}"
            .format(machine, machine_series, expected))


def setup(client, series):
    dummy_sink = local_charm_path(
        charm='charms/dummy-sink',
        juju_ver=client.version,
        series=series,
        repository=os.environ['JUJU_REPOSITORY'])
    dummy_subordinate = local_charm_path(
        charm='charms/dummy-subordinate',
        juju_ver=client.version,
        series=series,
        repository=os.environ['JUJU_REPOSITORY'])
    _, complete_primary = client.deploy(dummy_sink, series=series)
    _, complete_subordinate = client.deploy(dummy_subordinate, series=series)
    client.juju('add-relation', ('dummy-sink', 'dummy-subordinate'))
    client.set_config('dummy-subordinate', {'token': 'Canonical'})
    client.wait_for(complete_primary)
    client.wait_for(complete_subordinate)


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju update series.")
    add_basic_testing_arguments(parser)
    parser.add_argument('--from-series', default=DEFAULT_FROM_SERIES,
                        dest='from_series',
                        help='Series to start machine and units with')
    parser.add_argument('--to-series', default=DEFAULT_TO_SERIES,
                        dest='to_series',
                        help='Series to upgrade machine and units to')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        setup(bs_manager.client, args.from_series)
        assess_juju_upgrade_series(bs_manager.client, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
