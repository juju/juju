#!/usr/bin/env python

""" Assess using bundle that have various charms with lxd-profiles, testing
    different deployment scenarios.
"""

import argparse
import logging
import os
import sys

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

log = logging.getLogger("assess_lxdprofile_charm")

def deploy_bundle(client, charm_bundle):
    """Deploy the given charm bundle
    :param client: Jujupy ModelClient object
    :param charm_bundle: Optional charm bundle string
    """
    bundle = local_charm_path(
        charm=charm_bundle,
        juju_ver=client.version,
        repository=os.environ['JUJU_REPOSITORY']
    )
    _, primary = client.deploy(bundle)
    client.wait_for(primary)

def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test juju lxd profile bundle deploys.")
    parser.add_argument(
        '--charm-bundle',
        help="Override the charm bundle to deploy",
    )
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client

        deploy_bundle(client, charm_bundle=args.charm_bundle)

if __name__ == '__main__':
    sys.exit(main())