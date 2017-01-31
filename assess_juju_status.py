#!/usr/bin/env python
"""
    Deploy a charm and verify juju-status of the deployed application.
    Usage:
        python assess_juju_status.py [ --charm-app <charm-name> ]

    Example:
    1) To deploy default dummy-source charm
        python assess_juju_status.py
    2) To deploy charm of user choice
        python assess_juju_status.py --charm-app mysql

    NOTE: Currently assess_juju_status looks only for "juju-status" of
    the deployed application under application-<charm-name>-units
"""

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
from jujucharm import (
    local_charm_path,
    )
from assess_min_version import (
    JujuAssertionError
    )

__metaclass__ = type


log = logging.getLogger("assess_juju_status")


def verify_application_status(client, charm_app="dummy-source"):
    """
    Verify the deployed charm application status
    :param client: Juju client
    :param charm_app: String representing the default application name
    """
    try:
        output = client.get_status().status['applications'][charm_app]
        app_status = output['units'][charm_app + '/0']['juju-status']
        if not app_status:
            raise JujuAssertionError("application status not found")
        else:
            log.info("verified juju application status successfully")
    except KeyError:
        raise ValueError("Attribute not found")


def deploy_charm_app(client, charm_app="dummy-source", series="xenial"):
    """
    Deploy the charm
    :param client: Juju client
    :param charm_app: The charm app to be deployed
    :param series: The charm series to deploy
    :return:
    """
    charm_source = local_charm_path(
        charm=charm_app, juju_ver=client.version, series=series)
    client.deploy(charm_source)
    client.wait_for_started()
    log.info("Charm {} deployed successfully".format(charm_app))


def assess_juju_status(client, charm_app="dummy-source", series="xenial"):
    """
       Deploy specified charm app and verify the application-status
       :param client: Juju client
       :param charm_app: The name of the charm app to be deployed
       :param series: The charm series to deploy
    """
    deploy_charm_app(client, charm_app, series)
    verify_application_status(client, charm_app)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Juju application-status check")
    add_basic_testing_arguments(parser)
    parser.add_argument('--charm-app', action='store', default="dummy-source",
                        help='The charm to be deployed.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    series = args.series if args.series else 'xenial'
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_juju_status(bs_manager.client, args.charm_app, series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
