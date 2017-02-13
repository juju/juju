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


def verify_app_status(charm_details):
    """
    Verify the deployed charm application status
    :param charm_details: Deployed charm application details
    """

    app_status = charm_details.get('units').get('dummy-sink/0').get(
        'juju-status')
    if not app_status:
        raise JujuAssertionError("charm app status not found")
    else:
        log.info("verified charm app status successfully")


def verify_subordinate_app_status(charm_details):
    """
    Verify the deployed subordinate charm application status
    :param charm_details: Deployed charm application details
    """
    sub_status = charm_details.get('units').get('dummy-sink/0').get(
        'subordinates').get('dummy-subordinate/0').get('juju-status')
    if not sub_status:
        raise JujuAssertionError("charm subordinate status not found")
    else:
        log.info("verified charm subordinate status successfully")


def verify_charm_status(client):
    charm_details = client.get_status().get_applications()['dummy-sink']
    if charm_details:
        verify_app_status(charm_details)
        verify_subordinate_app_status(charm_details)
    else:
        raise JujuAssertionError("Failed to get charm details")


def deploy_charm_app(client, series):
    """
    Deploy dummy charm and dummy subordinate charm from local
    repository
    option
    :param client: Juju client
    :param series: The charm series to deploy
    """
    token = "canonical"
    charm_sink = local_charm_path(
        charm='dummy-sink', series=series, juju_ver=client.version)
    client.deploy(charm_sink)
    client.wait_for_started()
    charm_subordinate = local_charm_path(
        charm='dummy-subordinate', series=series, juju_ver=client.version)
    client.deploy(charm_subordinate)
    client.wait_for_started()
    client.set_config('dummy-subordinate', {'token': token})
    client.juju('add-relation', ('dummy-subordinate', 'dummy-sink'))
    client.juju('expose', ('dummy-sink',))


def assess_juju_status(client, series):
    """
       Deploy charm and subordinate charm and verify there status
       :param client: Juju client
       :param series: The charm series to deploy
    """
    deploy_charm_app(client, series)
    verify_charm_status(client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Juju application-status check")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    series = args.series if args.series else 'xenial'
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_juju_status(bs_manager.client, series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
