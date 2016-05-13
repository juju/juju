#!/usr/bin/env python
"""This testsuite is intended to test basic user permissions. Users
   can be granted read or full privileges by model. Revoking those
   privileges should remove them.

   A read permission user can see things such as status and
   perform read-only commands. A write permission user has
   equivalent powers as an admin"""

from __future__ import print_function

import argparse
from collections import namedtuple
import logging
import subprocess
import sys

import pexpect

from deploy_stack import (
    BootstrapManager,
)

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
)

__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")


# This needs refactored out to utility
class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


def register_user(username, environ, register_cmd,
                  register_process=pexpect.spawn):
    # needs support to passing register command with arguments
    # refactor once supported, bug 1573099
    try:
        child = register_process(register_cmd, env=environ)
        child.expect('(?i)name .*: ')
        child.sendline(username + '_controller')
        child.expect('(?i)password')
        child.sendline(username + '_password')
        child.expect('(?i)password')
        child.sendline(username + '_password')
        child.expect(pexpect.EOF)
        if child.isalive():
            raise JujuAssertionError(
                'Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
        raise JujuAssertionError(
            'Registering user failed: pexpect session timed out')


def create_cloned_environment(client, cloned_juju_home):
    user_client = client.clone(env=client.env.clone())
    user_client.env.juju_home = cloned_juju_home
    user_client_env = user_client._shell_environ()
    return user_client, user_client_env


def assert_read(client, permission):
    if permission:
        try:
            client.show_status()
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'User could not check status with read permission')
    else:
        try:
            client.show_status()
        except subprocess.CalledProcessError:
            pass
        else:
            raise JujuAssertionError(
                'User checked status without read permission')


def assert_write(client, permission):
    if permission:
        try:
            client.deploy('cs:ubuntu')
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'User could not deploy with write permission')
    else:
        try:
            client.deploy('cs:ubuntu')
        except subprocess.CalledProcessError:
            pass
        else:
            raise JujuAssertionError('User deployed without write permission')


def assert_user_permissions(user, user_client, admin_client):
    expect = iter(user.expect)
    assert_read(user_client, expect.next())
    assert_write(user_client, expect.next())

    log.debug("Revoking %s permission from %s" % (user.permissions, user.name))
    admin_client.revoke(user.name, permissions=user.permissions)

    assert_read(user_client, expect.next())
    assert_write(user_client, expect.next())


def assess_user_grant_revoke(admin_client):
    # Wait for the deployment to finish.
    admin_client.wait_for_started()

    log.debug("Creating Users")
    user = namedtuple('user', ['name', 'permissions', 'expect'])
    read_user = user('readuser', 'read', [True, False, False, False])
    write_user = user('adminuser', 'write', [True, True, True, False])
    users = [read_user, write_user]

    for user in users:
        log.debug("Testing %s" % user.name)
        user_register_string = admin_client.add_user(
            user.name, permissions=user.permissions)
        with temp_dir() as fake_home:
            user_client, user_env = create_cloned_environment(
                admin_client, fake_home)
            register_user(user.name, user_env, user_register_string)
            assert_user_permissions(user, user_client, admin_client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test grant and revoke permissions for users")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(logging.DEBUG)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_user_grant_revoke(bs_manager.client)
    return 0

if __name__ == '__main__':
    sys.exit(main())
