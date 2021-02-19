#!/usr/bin/env python3
"""Tests for the unregister command

Ensure that a users controller can be unregistered.

Test plan:
 - add user
 - run juju register command from previous step
 - verify controller shows up in juju list-controllers
 - 'juju unregister' the controller
 - verify the controller is no longer listed in juju list-controllers
 - verify that juju switch does not show the unregistered controller as
   the current controller; will show "ERROR no currently specified model"

"""

from __future__ import print_function

import argparse
import json
import logging
import subprocess
import sys
from textwrap import dedent

from assess_user_grant_revoke import (
    User,
)
from deploy_stack import (
    BootstrapManager,
)
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_unregister")


def assess_unregister(client):
    user = User('testuser', 'read', [])
    user_controller_name = '{}_controller'.format(user.name)
    with temp_dir() as fake_home:
        user_client = client.register_user(user, fake_home)
        assert_controller_list(
            user_client,
            [user_controller_name])

        user_client.juju(
            'unregister', ('--yes', user_controller_name), include_e=False)

        assert_controller_list(user_client, [])

        assert_switch_raises_error(user_client)

    # Ensure original controller still exists.
    assert_controller_list(client, [client.env.controller.name])


def assert_switch_raises_error(client):
    try:
        client.get_raw_juju_output('switch', None, include_e=False)
    except subprocess.CalledProcessError as e:
        if 'no model name was passed' not in e.stderr:
            raise JujuAssertionError(
                '"juju switch" command failed for an unexpected reason: '
                '{}'.format(e.stderr))
        log.info('"juju switch" failed as expected')
        return
    raise JujuAssertionError('"juju switch failed to error as expected."')


def assert_controller_list(client, controller_list):
    """Assert that clients controller list only contains names provided.

    :param client: ModelClient to retrieve controllers of.
    :param controller_list: list of strings for expected controller names.

    """
    json_output = client.get_juju_output(
        'list-controllers', '--format', 'json', include_e=False)
    output = json.loads(json_output)

    try:
        controller_names = list(output['controllers'].keys())
    except AttributeError:
        # It's possible that there are 0 controllers for this client.
        controller_names = []

    if controller_names != controller_list:
        raise JujuAssertionError(
            dedent("""\
            Unexpected controller names.
            Expected: {}
            Got: {}""".format(
                controller_list, controller_names)))


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test unregister feature.")
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_unregister(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
