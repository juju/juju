#!/usr/bin/env python
"""Assess Juju usage of the staging charm store."""

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


log = logging.getLogger("assess_cs_staging")


def _get_ssh_script(ip):
    return (
        '''sudo bash -c "echo '%s store.juju.ubuntu.com' >> /etc/hosts"'''
        % ip)


def _set_charm_store_ip(client, ip):
    client.get_admin_client().juju('ssh', ('0', _get_ssh_script(ip)))


def assess_deploy(client, charm):
    """Deploy the charm."""
    client.deploy(charm)
    client.wait_for_started()
    log.info("Deploying charm %r and waiting for started.", charm)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test staging store.")
    parser.add_argument('charm_store_ip', help="Charm store address.")
    add_basic_testing_arguments(parser)
    parser.add_argument('--charm', default='ubuntu', help='Charm to deploy.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        _set_charm_store_ip(bs_manager.client, args.charm_store_ip)
        assess_deploy(bs_manager.client, args.charm)
    return 0


if __name__ == '__main__':
    sys.exit(main())
