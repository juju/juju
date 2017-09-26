#!/usr/bin/env python
"""This test will test the juju sla command. Currently, testing budget or
supported sla models requires a functioning omnibus. As such, the current
test merely checks to ensure unsupported models appear as unsupported"""

from __future__ import print_function

import argparse
import logging
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


log = logging.getLogger("assess_sla")


def list_sla(client):
    """Return output of the sla command.

    This will return the support level for a model"""
    return client.get_juju_output('sla', include_e=False).strip()


def assert_sla_state(client, expected_state):
    sla_state = list_sla(client)
    if expected_state not in sla_state:
        raise JujuAssertionError(
            'Found: {}\nExpected: {}'.format(sla_state, expected_state))


def assess_sla(client, series='xenial'):
    client.wait_for_started()
    dummy_source = local_charm_path(charm='dummy-source',
                                    juju_ver=client.version,
                                    series=series)
    client.deploy(charm=dummy_source)
    client.wait_for_workloads()
    # As we are unable to test supported models, for now, we only can assert
    # on the model shows correctly as unsupported
    assert_sla_state(client, 'unsupported')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test the juju sla command")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_sla(bs_manager.client, args.series)
    return 0


if __name__ == '__main__':
    sys.exit(main())
