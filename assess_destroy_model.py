#!/usr/bin/env python
"""Assess if Juju tracks the model when the current model is destroyed."""

from __future__ import print_function

import argparse
import logging
import sys
import subprocess
import json

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
    controller = get_current_controller(client)
    log.info('Current model: {}'.format(current_model))

    new_client = add_model(client)
    destroy_model(client, new_client)

    log.info('Juju successfully dropped its current model. '
             'Switching to {} to complete test'.format(current_model))
    switch_model(client, current_model, controller)

    log.info('SUCCESS')


def add_model(client):
    """Adds a model to the current juju environment then destroys it.

    Will raise an exception if the Juju does not deselect the current model.

    :param client: Jujupy ModelClient object
    """
    log.info('Adding model "{}" to current controller'.format(TEST_MODEL))
    new_client = client.add_model(TEST_MODEL)
    new_model = get_current_model(new_client)
    if new_model == TEST_MODEL:
        log.info('Current model and newly added model match')
    else:
        error = ('Juju failed to switch to new model after creation. '
                 'Expected {} got {}'.format(TEST_MODEL, new_model))
        raise JujuAssertionError(error)
    return new_client


def destroy_model(client, new_client):
    log.info('Destroying model "{}"'.format(TEST_MODEL))
    new_client.destroy_model()
    new_model = get_current_model(client)
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
    raw = client.get_juju_output('switch', include_e=False).decode('utf-8')
    raw = raw.split(':')[0]
    return raw


def get_current_model(client):
    """Gets the current model from Juju's list-models command.

    :param client: Jujupy ModelClient object
    :return: String name of current model
    """
    raw = list_models(client)
    try:
        return raw['current-model']
    except KeyError:
        log.warning('No model is currently selected.')
        return None


def list_models(client):
    """Helper function to get the output of juju's list-models command.

    Instead of using 'juju switch' or client.backend.get_active_model() we use
    list-models because that was what the bug report this test was generated
    around used. It also allows for flexiblity in the future to get more
    detailed information about the models that Juju thinks it has
    if we need it.

    :param client: Jujupy ModelClient object
    :return: Dict of list-models command
    """
    try:
        raw = client.get_juju_output('list-models', '--format', 'json',
                                     include_e=False).decode('utf-8')
    except subprocess.CalledProcessError as e:
        log.error('Failed to list current models due to error: {}'.format(e))
        raise e
    return json.loads(raw)


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
