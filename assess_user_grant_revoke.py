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
import copy
import json
import logging
import subprocess
import sys

import pexpect

from deploy_stack import (
    BootstrapManager,
)

from jujupy import Controller
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
)

__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")

User = namedtuple('User', ['name', 'permissions', 'expect'])


user_list_admin = [{"user-name": "admin", "display-name": "admin"}]
user_list_admin_read = copy.deepcopy(user_list_admin)
user_list_admin_read.append({"user-name": "readuser", "display-name": ""})
user_list_all = copy.deepcopy(user_list_admin_read)
user_list_all.append({"user-name": "adminuser", "display-name": ""})
share_list_admin = {"admin@local": {"display-name": "admin", "access": "admin"}}
share_list_admin_read = copy.deepcopy(share_list_admin)
share_list_admin_read["readuser@local"] = {"access": "read"}
share_list_all = copy.deepcopy(share_list_admin_read)
share_list_all["adminuser@local"] = {"access": "write"}
share_list_all.pop("readuser@local")


# This needs refactored out to utility
class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


def list_users(client):
    """Test listing all the users"""
    users_list = json.loads(client.get_juju_output('list-users', '--format',
                                                   'json', include_e=False))
    for user in users_list:
        """Pop date-created and last-connection if existed for comparison"""
        user.pop("date-created", None)
        user.pop("last-connection", None)
    return users_list


def list_shares(client):
    """Test listing users' shares"""
    share_list = json.loads(client.get_juju_output('list-shares', '--format',
                                                   'json', include_e=False))
    for key, value in share_list.iteritems():
        """Pop last-connection if existed for comparison"""
        value.pop("last-connection", None)
    return share_list


def show_user(client):
    """Test showing a user's status"""
    user_status = json.loads(client.get_juju_output('show-user', '--format',
                                                    'json', include_e=False))
    """Pop date-created and last-connection if existed for comparison"""
    user_status.pop("date-created", None)
    user_status.pop("last-connection", None)
    return user_status


def assign_user_name(client, user_name):
    """Assign the name of current user to the user Environment"""
    client.env.user_name = user_name


def register_user(user, client, fake_home):
    """Register `user` for the `client` return the cloned client used."""
    # needs support to passing register command with arguments
    # refactor once supported, bug 1573099
    username = user.name
    controller_name = '{}_controller'.format(username)

    token = client.add_user(username, permissions=user.permissions)
    user_client, user_env = create_cloned_environment(
        client, fake_home, controller_name)

    try:
        child = user_client.expect(
            'register', (token), extra_env=user_env, include_e=False)
        child.expect('(?i)name')
        child.sendline(username + '_controller')
        child.expect('(?i)password')
        child.sendline(username + '_password')
        child.expect('(?i)password')
        child.sendline(username + '_password')
        child.expect(pexpect.EOF)
        if child.isalive() is True:
            raise JujuAssertionError(
                'Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
        raise JujuAssertionError(
            'Registering user failed: pexpect session timed out')
    assign_user_name(user_client, username)
    return user_client


def create_cloned_environment(client, cloned_juju_home, controller_name):
    """Create a cloned environment"""
    user_client = client.clone(env=client.env.clone())
    user_client.env.juju_home = cloned_juju_home
    # New user names the controller.
    user_client.env.controller = Controller(controller_name)
    user_client_env = user_client._shell_environ()
    return user_client, user_client_env


def assert_read(client, permission):
    """Test if the user has or doesn't have the read permission"""
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
    """Test if the user has or hasn't the write permission"""
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


def assert_user_permissions(user, user_client, controller_client):
    """Test if users' permissions are within expectations"""
    expect = iter(user.expect)
    assert_read(user_client, expect.next())
    assert_write(user_client, expect.next())

    log.debug("Revoking %s permission from %s" % (user.permissions, user.name))
    controller_client.revoke(user.name, permissions=user.permissions)

    assert_read(user_client, expect.next())
    assert_write(user_client, expect.next())


def assert_users_shares(controller_client, client, user):
    """Test if user_list and share_list are expected"""
    if user.name == 'readuser':
        user_list_expected = user_list_admin_read
        share_list_expected = share_list_admin_read
    else:
        user_list_expected = user_list_all
        share_list_expected = share_list_all
    user_list = list_users(controller_client)
    share_list = list_shares(controller_client)
    if sorted(user_list) != sorted(user_list_expected):
        raise JujuAssertionError(user_list)
    if set(share_list) != set(share_list_expected):
        raise JujuAssertionError(share_list)


def assess_change_password(client, user, fake_home):
    """Test changing user's password"""
    username = user.name
    controller_name = '{}_controller'.format(username)
    user_client, user_env = create_cloned_environment(
        client, fake_home, controller_name)
    try:
        child = user_client.expect('change-user-password', (user.name,),
                                   extra_env=user_env, include_e=False)
        child.expect('(?i)password')
        child.sendline(user.name + '_password_2')
        child.expect('(?i)password')
        child.sendline(user.name + '_password_2')
        child.expect(pexpect.EOF)
        if child.isalive():
            raise JujuAssertionError(
                'Changing user password failed: pexpect session still alive')
    except pexpect.TIMEOUT:
        raise JujuAssertionError(
            'Changing user password failed: pexpect session timed out')


def assess_disable_enable(controller_client, users):
    """Test disabling and enabling users"""
    controller_client.disable_user(users[-1].name)
    user_list = list_users(controller_client)
    if sorted(user_list) != sorted(user_list_admin_read):
        raise JujuAssertionError(user_list)
    controller_client.enable_user(users[-1].name)
    user_list = list_users(controller_client)
    if sorted(user_list) != sorted(user_list_all):
        raise JujuAssertionError(user_list)


def assess_logout_login(controller_client, user_client, user, fake_home):
    """Test users' login and logout"""
    user_client.logout_user()
    user_list = list_users(controller_client)
    if sorted(user_list) != sorted(user_list_all):
        raise JujuAssertionError(user_list)
    username = user.name
    controller_name = '{}_controller'.format(username)
    client, user_env = create_cloned_environment(
        controller_client, fake_home, controller_name)
    try:
        child = client.expect('login', (user.name,),
                              extra_env=user_env, include_e=False)
        child.expect('(?i)password')
        child.sendline(user.name + '_password_2')
        child.expect(pexpect.EOF)
        if child.isalive():
            raise JujuAssertionError(
                'Login user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
        raise JujuAssertionError(
            'Login user failed: pexpect session timed out')


def assess_user_grant_revoke(controller_client):
    """Test multi-users functionality"""
    assign_user_name(controller_client, 'admin')
    log.debug("Creating Users")
    read_user = User('readuser', 'read', [True, False, False, False])
    write_user = User('adminuser', 'write', [True, True, True, False])
    users = [read_user, write_user]
    user_list = list_users(controller_client)
    share_list = list_shares(controller_client)
    user_status = show_user(controller_client)
    if sorted(user_list) != sorted(user_list_admin):
        raise JujuAssertionError(user_list)
    if share_list != share_list_admin:
        raise JujuAssertionError(share_list)
    if user_status != user_list_admin[0]:
        raise JujuAssertionError(user_status)
    for user in users:
        log.debug("Testing %s" % user.name)
        with temp_dir() as fake_home:
            user_client = register_user(
                user, controller_client, fake_home)
            assert_users_shares(controller_client, user_client, user)
            assess_change_password(user_client, user, fake_home)
            if user.name == 'adminuser':
                assess_disable_enable(controller_client, users)
                assess_logout_login(controller_client, user_client, user, fake_home)
            assert_user_permissions(user, user_client, controller_client)


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
