#!/usr/bin/env python
"""Assess Juju under various proxy network conditions."""

from __future__ import print_function

import argparse
import logging
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


def set_firewall(scenario):
    """Setup the firewall to match the scenario."""
    pass


def reset_firewall():
    """Reset the firewall and disable it."""
    pass


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
