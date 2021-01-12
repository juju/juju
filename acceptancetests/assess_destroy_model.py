#!/usr/bin/env python3
"""Assess if Juju tracks the model when the current model is destroyed."""

from __future__ import print_function

import argparse
import logging
import sys
import subprocess
import time

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    add_model,
    get_current_model,
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


def destroy_model(client, new_client):
    log.info('Destroying model "{}"'.format(TEST_MODEL))
    new_client.destroy_model()
    old_model = get_current_model(client)
    if not old_model:
        error = 'Juju unset model after it was destroyed'
        raise JujuAssertionError(error)
    for attempt in range(10):
        try:
            client.get_juju_output('status', include_e=False)
        except subprocess.CalledProcessError as e:
            not_found_error = b'{} not found'.format(old_model)
            if not_found_error in e.stderr:
                log.info("Model fully removed")
                break
            removed_error = b'model "admin/{}" has been removed from the controller'.format(old_model)
            if removed_error not in e.stderr:
                error = 'unexpected error calling status\n{}'.format(e.stderr)
                raise JujuAssertionError(error)
            log.info("Model reported as removed...")
        else:
            error = 'model still valid after it was destroyed'
            raise JujuAssertionError(error)
        time.sleep(10)
    else:
        # We didn't break out of the loop, so the model-removed message didn't
        # change to model not found (indicating that the model hasn't been
        # removed from the state pool).
        raise JujuAssertionError("didn't get not found error - model still in state pool")


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


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Test if juju drops selection of the current model '
        'when that model is destroyed.')
    add_basic_testing_arguments(parser, existing=False)
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
