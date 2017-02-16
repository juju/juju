#!/usr/bin/env python
"""
    Deploy a charm and subordinate charm and verify juju-status of the
    deployed charms.
    Usage:
        python assess_juju_status.py

    NOTE: Currently assess_juju_status looks only for "juju-status" attribute
    of the deployed application under application-<charm-name>-units
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


def verify_juju_status_attribute_of_charm(client):
    """Verify the juju-status of the deployed charm

    :param charm_details: Deployed charm application details
    """
    charm_details = client.get_status().get_applications()['dummy-sink']
    log.warning(charm_details)
    try:
        app_status = charm_details['units']['dummy-sink/0']['juju-status']
    except KeyError:
        raise ValueError("Attribute not found")
    if not app_status:
        raise JujuAssertionError("Charm App status is not set")


def verify_juju_status_attribute_of_subordinate_charm(client):
    """Verify the juju-status of deployed subordinate charm

    :param charm_details: Dictionary representing charm application details
    """
    charm_details = client.get_status().get_applications()['dummy-sink']
    log.warning(charm_details)
    try:
        sub_status = charm_details['units']['dummy-sink/0']['subordinates'][
            'dummy-subordinate/0']['juju-status']
    except KeyError:
        raise ValueError("Attribute not found")
    if not sub_status:
        raise JujuAssertionError("Charm Subordinate status is not set")


def deploy_charm_with_subordinate_charm(client, series):
    """Deploy dummy-sink charm and dummy-subordinate charm

    :param client: ModelClient object
    :param series: String representing charm series
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
    client.wait_for_workloads()


def assess_juju_status_attribute(client, series):
    """Deploy charm and subordinate charm and verify its juju-status attribute

    :param client: ModelClient object
    :param series: String representing charm series
    """
    deploy_charm_with_subordinate_charm(client, series)
    verify_juju_status_attribute_of_charm(client)
    log.warning("Completed - verify_juju_status_attribute_of_charm")
    verify_juju_status_attribute_of_subordinate_charm(client)
    log.warning("assess juju-status attribute done successfully")


def parse_args(argv):
    """Parse all arguments."""

    parser = argparse.ArgumentParser(
        description="Test juju-status of charm and its subordinate charm")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    series = args.series if args.series else 'xenial'
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_juju_status_attribute(bs_manager.client, series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
