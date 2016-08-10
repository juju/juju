#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""

from __future__ import print_function

import argparse
import logging
import random
import string
import sys

from assess_user_grant_revoke import (
    assert_change_password,
    assert_logout_login,
    User,
)
from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_controller_permissions")


# This needs refactored out to utility
class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


def assert_add_model(user_client, permission):
    try:
        user_client.add_model(user_client.env)
    except subprocess.CalledProcessError:
        raise JujuAssertionError(
            "Controller can't add model with {} permission".format(permission))


def assert_destroy_model(user_client, permission):
    try:
        user_client.destroy_model()
    except subprocess.CalledProcessError:
        raise JujuAssertionError(
            "Controller can't destroy model with {} permission".format(permission))


def assert_add_remove_user(user_client, permission):
    for controller_permission in ['login', 'addmodel', 'superuser']:
        code = ''.join(random.choice(
            string.ascii_letters + string.digits) for _ in xrange(4))
        try:
            user_client.add_user(permission + code, permissions=controller_permission)
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'Controller could not add {} controller with {} permission'.format(
                    controller_permission, permission))
        try:
            user_client.remove_user(permission + code, permissions=controller_permission)
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'Controller could not remove {} controller with {} permission'.format(
                    controller_permission, permission))

def assert_login_controller(controller_client, user):
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        assert_logout_login(controller_client, user_client, user, fake_home)
        assert_change_password(user_client, user)


def assert_addmodel_controller(controller_client, user):
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        assert_add_model(user_client, user.permissions)
        assert_destroy_model(user_client, user.permissions)


def assert_superuser_controller(controller_client, user):
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        assert_add_remove_user(user_client, user.permissions)


def assess_controller_permissions(controller_client):
    login_controller = User('login_controller', 'login', [])
    addmodel_controller = User('addmodel_controller', 'addmodel', [])
    superuser_controller = User('superuser_controller', 'superuser', [])
    assert_login_controller(controller_client, login_controller)
    assert_addmodel_controller(controller_client, addmodel_controller)
    assert_superuser_controller(controller_client, superuser_controller)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test controller permissions.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_controller_permissions(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
