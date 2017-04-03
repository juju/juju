#!/usr/bin/python3
"""Assess if Juju tracks the controller when the current model is destroyed."""

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


log = logging.getLogger("assess_destroy_model")


def assess_destroy_model(client):
    """
    Grab current controller from client.
    Add a model, check if the controller stays the same.
    Destroy that model, check if the controller stays the same.
    Teardown
    """
    current_controller_id = client.get_status().status['model']['controller']
    log.info('Current controller ID: {}'.format(current_controller_id))

    log.info('Adding model "test" to current controller.')
    client.add_model('test')
    log.info('Destroying model "test"')
    client.destroy_model(model='test')
    new_controller_id = client.get_status().status['model']['controller']
    log.info('Controller ID after destroy: {}'.format(new_controller_id))
    assert (current_controller_id == new_controller_id)
    log.info('SUCCESS')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Test if juju keeps track of the current controller '
        'when a model is destroyed.')
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_destroy_model(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
