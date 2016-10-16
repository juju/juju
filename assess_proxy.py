#!/usr/bin/env python
"""Assess Juju under various proxy network conditions."""

from __future__ import print_function

import argparse
import logging
import subprocess
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_proxy")


UFW_RESET_COMMANDS = [
    ('sudo', 'ufw', '--force', 'reset'),
    ('sudo', 'ufw', '--force', 'disable'),
]


def set_firewall(scenario):
    """Setup the firewall to match the scenario."""
    pass


def reset_firewall():
    """Reset the firewall and disable it.

    The firewall's rules are reset, then it is disabled. The ufw reset command
    implicitly disables, but disable is explicitly called to ensure ufw
    is not running.
    """
    for command in UFW_RESET_COMMANDS:
        exitcode = subprocess.call(command)
        if exitcode == 0:
            log.info('{} exited successfully'.format(command))
        else:
            log.error('{} exited with {}'.format(command, exitcode))
            log.error('This host may be in a dirty state.')


def assess_proxy(client, scenario):
    client.deploy('cs:xenial/ubuntu')
    client.wait_for_started()
    client.wait_for_workloads()
    log.info("SUCCESS")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Assess Juju under various proxy network conditions.")
    add_basic_testing_arguments(parser)
    parser.add_argument(
        'scenario', choices=['both-proxied'],
        help="The proxy scenario to run.")
    parser.add_argument(
        '--client-interface', default='eth0',
        help="The interface used by the client to access the internet.")
    parser.add_argument(
        '--controller-interface', default='lxdbr0',
        help="The interface used by the controller to access the internet.")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    try:
        log.info("Setting firewall")
        set_firewall(args.scenario)
        log.info("Starting test")
        bs_manager = BootstrapManager.from_args(args)
        log.info("Starting bootstrap")
        with bs_manager.booted_context(args.upload_tools):
            log.info("PASS bootstrap")
            assess_proxy(bs_manager.client, args.scenario)
            log.info("Finished test")
    finally:
        # Always open the network, regardless of what happened.
        # Do not lockout the host.
        log.info("Resetting firewall")
        reset_firewall()
    return 0


if __name__ == '__main__':
    sys.exit(main())
