#!/usr/bin/python3
"""Assess if Juju tracks the controller when the current model is destroyed."""

from __future__ import print_function

import argparse
import logging
import sys
import subprocess
import re

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )


__metaclass__ = type


log = logging.getLogger("assess_destroy_model")

TEST_MODEL = 'test-tmp-env'


def assess_destroy_model(client):
    """Tests if Juju tracks the model properly through deletion.

    In normal behavior Juju should drop the current model selection if that
    model is destroyed. This will fail if Juju does not drop it's current
    selection.

    :param client: Jujupy ModelClient object
    """

    current_model = get_current_model(client)
    current_controller = get_current_controller(client)
    log.info('Current model: {}'.format(current_model))

    new_client = add_model(client)
    destroy_model(new_client)
    log.info('Juju successfully dropped its current model. '
             'Switching to old model to complete test')

    switch_model(client, current_model, current_controller)

    log.info('SUCCESS')


def add_model(client):
    """Adds a model to the current juju environment then destroys it.

    Will raise an exception if the Juju does not deselect the current model.

    :param client: Jujupy ModelClient object
    """
    log.info('Adding model "tested" to current controller')
    new_client = client.add_model(TEST_MODEL)
    new_model = get_current_model(new_client)
    if new_model == TEST_MODEL:
        log.info('Current model and newly added model match')
    else:
        error = ('Juju failed to switch to new model after creation. '
                 'Expected {} got {}'.format(TEST_MODEL, new_model))
        raise JujuAssertionError(error)
    return new_client


def destroy_model(new_client):
    log.info('Destroying model "{}"'.format(TEST_MODEL))
    new_client.destroy_model()
    new_model = get_current_model(new_client)
    if new_model:
        error = 'Juju failed to unset model after it was destroyed'
        raise JujuAssertionError(error)


def switch_model(client, current_model, current_controller):
    """Switches back to the old model.

    :param client: Jujupy ModelClient object
    :param current_model: String name of initial testing model
    :param current_controller: String name of testing controller
    """
    client.switch(model=current_model, controller=current_controller)
    new_model = get_current_model(client)
    if new_model == current_model:
        log.info('Current model and switch target match')
    else:
        error = ('Juju failed to switch back to existing model. '
                 'Expected {} got {}'.format(TEST_MODEL, new_model))
        raise JujuAssertionError(error)


def get_current_controller(client):
    """Gets the current controller from Juju's list-models command.

    :param client: Jujupy ModelClient object
    :return: String name of current controller
    """
    raw = list_models(client)
    pattern = r".*Controller:\s*([a-z0-9'-]+)"
    match = re.search(pattern, raw)
    if match:
        return match.group(1)
    else:
        error = ('Failed to get current controller')
        raise JujuAssertionError(error)


def get_current_model(client):
    """Gets the current model from Juju's list-models command.

    :param client: Jujupy ModelClient object
    :return: String name of current model
    """
    raw = list_models(client)
    pattern = r"([a-z0-9'-]+)\*"
    match = re.search(pattern, raw)
    if match:
        return match.group(1)
    else:
        log.warning('Could not get current model.')
        return None


def list_models(client):
    """Helper function to get the output of juju's list-models command.

    :param client: Jujupy ModelClient object
    :return: Utf-8 formatted string of list-models command
    """
    try:
        raw = client.get_juju_output('list-models',
                                     include_e=False).decode("utf-8")
    except subprocess.CalledProcessError as e:
        log.error('Failed to list current models due to error: {}'.format(e))
        raise e
    return raw


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Test if juju drops selection of the current model '
        'when that model is destroyed.')
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
