#!/usr/bin/env python

""" Assess deployments of bundles requiring trust when --trust is passed to
    juju deploy. The trust-checker charm used by this test will set its status
    to blocked unless it can succesfully access cloud credentials after being
    granted trust (either by the operator or the juju deploy command)
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
)
from jujupy.wait_condition import AllApplicationActive

__metaclass__ = type
log = logging.getLogger("assess_deploy_trusted_bundle")
default_bundle = 'bundles-trust-checker.yaml'


def deploy_bundle(client, charm_bundle):
    """Deploy the given charm bundle
    :param client: Jujupy ModelClient object
    :param charm_bundle: Optional charm bundle string
    """
    if not charm_bundle:
        bundle = local_charm_path(
            charm=default_bundle,
            juju_ver=client.version,
            repository=os.environ['JUJU_REPOSITORY']
        )
    else:
        bundle = charm_bundle
    # bump the timeout of the wait_timeout to ensure that we can give more time
    # for complex deploys. After deploying wait up to 60sec for the
    # applications to register as active.
    _, primary = client.deploy(bundle, wait_timeout=1800, trust="true")
    client.wait_for(AllApplicationActive(1800))


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description="Test trusted bundle deployments")
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
