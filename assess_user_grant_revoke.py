#!/usr/bin/env python
"""This testsuite is intended to test basic user permissions. Users
   can be granted read or full privileges by model. Revoking those
   privileges should remove them.

   A read permission user can see things such as status and
   perform read-only commands. A write permission user has
   equivalent powers as an admin"""

from __future__ import print_function

import argparse
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


def register_user(username, environment, register_cmd,
                  register_process=pexpect.spawn):
    # needs support to passing register command with arguments
    # refactor once supported, bug 1573099
    try:
        child = register_process(register_cmd, env=environment)
        child.expect('(?i)name .*: ')
        child.sendline(username + '_controller')
        child.expect('(?i)password')
        child.sendline(username + '_password')
        child.expect('(?i)password')
        child.sendline(username + '_password')
        child.expect(pexpect.EOF)
        if child.isalive():
            raise AssertionError(
                'Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
        raise AssertionError(
            'Registering user failed: pexpect session timed out')

def create_cloned_environment(client, cloned_juju_home):
    user_client = client.clone(env=client.env.clone())
    user_client.env.juju_home = cloned_juju_home
    user_client_env = user_client._shell_environ()
    return user_client, user_client_env

def assert_read(client, permission):
    if permission is True:
        self.assertTrue(client.show_user())
        self.assertTrue(client.list_controllers())
        self.assertTrue(client.show_status())
    else:
        self.assertFalse(client.show_user())
        self.assertFalse(client.list_controllers())
        self.assertFalse(client.show_status())

def assert_write(client, permission):
    if permission is True:
        self.assertTrue(client.deploy('local:wordpress')
    else:
        self.assertFalse(client.deploy('local:wordpress')


def assess_user_grant_revoke(client):
    # Wait for the deployment to finish.
    client.wait_for_started()

    log.debug("Creating Users")
    read_user = 'bob'
    write_user = 'carol'
    read_user_register = client.create_user_permissions(read_user)
    write_user_register = client.create_user_permissions(write_user)

    log.debug("Testing read_user access")
    with temp_dir() as fake_home:
        read_user_client, read_user_env = create_cloned_environment(
            client, fake_home)
        register_user(read_user, read_user_env, read_user_register)

        assert_read(read_user, True)
        assert_write(read_user, False)

        # remove all permissions
        log.debug("Revoking permissions from read_user")
        client.remove_user_permissions(client, read_user)

        assert_read(read_user, False)
        assert_write(read_user, False)

    log.debug("Testing write_user access")
    with temp_dir() as fake_home:
        write_user_client, write_user_env = create_cloned_environment(
            client, fake_home)
        register_user(write_user, write_user_env, write_user_register)

        assert_read(read_user, True)
        assert_write(read_user, True)

        # remove all permissions
        log.debug("Revoking permissions from write_user")
        write_user_client.remove_user_permissions(write_user, permissions='write')

        assert_read(read_user, True)
        assert_write(read_user, False)


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
