#!/usr/bin/env python3
"""
    Deploy a charm and subordinate charm and verify juju-status attribute of
    the deployed charms.
    Usage:
        python assess_juju_output.py

    NOTE: Currently assess_juju_output looks only for "juju-status" attribute
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


log = logging.getLogger("assess_juju_output")

def verify_juju_status_contains_display_name(client_status):
    fields_to_test = [
        "instance-id",
        "display-name",
    ]
    for field in fields_to_test:
        try:
            client_status[field]
        except KeyError:
            err = "juju status excludes {}".format(field)
            raise JujuAssertionError(err)

def verify_juju_status_attribute_of_charm(charm_details):
    """Verify the juju-status of the deployed charm

    :param charm_details: Deployed charm application details
    """
    try:
        app_status = charm_details['units']['dummy-sink/0']['juju-status']
    except KeyError:
        raise JujuAssertionError(
            "juju-status for dummy-sink was not found")
    if not app_status:
        raise JujuAssertionError(
            "App status for dummy-sink is not set")


def verify_juju_status_attribute_of_subordinate_charm(charm_details):
    """Verify the juju-status of deployed subordinate charm

    :param charm_details: Dictionary representing charm application details
    """
    try:
        sub_status = charm_details['units']['dummy-sink/0']['subordinates'][
            'dummy-subordinate/0']['juju-status']
    except KeyError:
        raise JujuAssertionError(
            "juju-status for dummy-subordinate was not found")
    if not sub_status:
        raise JujuAssertionError(
            "App status for dummy-subordinate is not set")


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


def assess_juju_status(client, series):
    """Deploy charm and subordinate charm and verify its juju-status attribute

    :param client: ModelClient object
    :param series: String representing charm series
    """
    deploy_charm_with_subordinate_charm(client, series)
    status = client.get_status()
    verify_juju_status_fields(status)
    charm_details = status.get_applications()['dummy-sink']
    verify_juju_status_attribute_of_charm(charm_details)
    verify_juju_status_attribute_of_subordinate_charm(charm_details)
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
        assess_juju_status(bs_manager.client, series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
