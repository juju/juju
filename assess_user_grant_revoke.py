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
import random
import string
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
    wait_for_removed_services,
)

__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")

User = namedtuple('User', ['name', 'permissions', 'expect'])


user_list_admin = [{"user-name": "admin", "display-name": "admin"}]
user_list_admin_read = copy.deepcopy(user_list_admin)
# Created user has no display name, bug 1606354
user_list_admin_read.append({"user-name": "readuser", "display-name": ""})
user_list_admin_read_write = copy.deepcopy(user_list_admin_read)
# bug 1606354
user_list_admin_read_write.append({"user-name": "writeuser",
                                   "display-name": ""})
user_list_all = copy.deepcopy(user_list_admin_read_write)
# bug 1606354
user_list_all.append({"user-name": "adminuser", "display-name": ""})
share_list_admin = {"admin@local": {"display-name": "admin",
                                    "access": "admin"}}
share_list_admin_read = copy.deepcopy(share_list_admin)
share_list_admin_read["readuser@local"] = {"access": "read"}
share_list_admin_read_write = copy.deepcopy(share_list_admin_read)
share_list_admin_read_write["writeuser@local"] = {"access": "write"}
del share_list_admin_read_write['readuser@local']
share_list_all = copy.deepcopy(share_list_admin_read_write)
share_list_all["adminuser@local"] = {"access": "admin"}
share_list_all["writeuser@local"]["access"] = "read"


# This needs refactored out to utility
class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


def list_users(client):
    """Test listing all the users"""
    users_list = json.loads(client.get_juju_output('list-users', '--format',
                                                   'json', include_e=False))
    for user in users_list:
        # Pop date-created and last-connection if existed for comparison
        user.pop("date-created", None)
        user.pop("last-connection", None)
    return users_list


def list_shares(client):
    """Test listing users' shares"""
    share_list = json.loads(client.get_juju_output('list-shares', '--format',
                                                   'json', include_e=False))
    for key, value in share_list.iteritems():
        # Pop last-connection if existed for comparison
        value.pop("last-connection", None)
    return share_list


def show_user(client):
    """Test showing a user's status"""
    user_status = json.loads(client.get_juju_output('show-user', '--format',
                                                    'json', include_e=False))
    # Pop date-created and last-connection if existed for comparison
    user_status.pop("date-created", None)
    user_status.pop("last-connection", None)
    return user_status


def assert_read_model(client, permission, has_permission):
    """Test if the user has or doesn't have the read permission"""
    if has_permission:
        try:
            client.show_status()
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'User could not check status with {} permission'.format(
                    permission))
    else:
        try:
            client.show_status()
        except subprocess.CalledProcessError:
            pass
        else:
            raise JujuAssertionError(
                'User checked status without {} permission'.format(permission))


def assert_write_model(client, permission, has_permission):
    """Test if the user has or doesn't have the write permission"""
    if has_permission:
        try:
            client.deploy('cs:ubuntu')
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'User could not deploy with {} permission'.format(permission))
        else:
            client.remove_service('ubuntu')
            client.wait_for_started()
    else:
        try:
            client.deploy('cs:ubuntu')
        except subprocess.CalledProcessError:
            pass
        else:
            raise JujuAssertionError(
                'User deployed without {} permission'.format(permission))


def assert_admin_model(controller_client, client, permission, has_permission):
    """Test if the user has or doesn't have the admin permission"""
    code = ''.join(random.choice(
        string.ascii_letters + string.digits) for _ in xrange(4))
    new_user = permission + code
    controller_client.add_user(new_user, permissions="read")
    if has_permission:
        try:
            client.grant(new_user, permission="write")
        except subprocess.CalledProcessError:
            raise JujuAssertionError(
                'User could not grant write access to user with {} permission'.format(
                    permission))
    else:
        try:
            client.grant(new_user, permission="write")
        except subprocess.CalledProcessError:
            pass
        else:
            raise JujuAssertionError(
                'User granted access without {} permission'.format(permission))


def assert_user_permissions(user, user_client, controller_client):
    """Test if users' permissions are within expectations"""
    expect = iter(user.expect)
    permission = user.permissions
    assert_read_model(user_client, permission, expect.next())
    assert_write_model(user_client, permission, expect.next())
    assert_admin_model(controller_client, user_client, permission, expect.next())

    log.debug("Revoking %s permission from %s" % (user.permissions, user.name))
    controller_client.revoke(user.name, permissions=user.permissions)

    assert_read_model(user_client, permission, expect.next())
    assert_write_model(user_client, permission, expect.next())
    assert_admin_model(controller_client, user_client, permission, expect.next())


def assert_users_shares(controller_client, client, user):
    """Test if user_list and share_list are expected"""
    if user.name == 'readuser':
        user_list_expected = user_list_admin_read
        share_list_expected = share_list_admin_read
    else:
        user_list_expected = user_list_admin_read_write
        share_list_expected = share_list_all
    user_list = list_users(controller_client)
    share_list = list_shares(controller_client)
    if sorted(user_list) != sorted(user_list_expected):
        raise JujuAssertionError(user_list)
    if sorted(share_list) != sorted(share_list_expected):
        raise JujuAssertionError(share_list)


def assert_change_password(client, user):
    """Test changing user's password"""
    try:
        child = client.expect('change-user-password', (user.name,),
                              include_e=False)
        child.expect('(?i)password')
        child.sendline(user.name + '_password_2')
        child.expect('(?i)password')
        child.sendline(user.name + '_password_2')
        child.expect(pexpect.EOF)
    except pexpect.TIMEOUT:
        raise JujuAssertionError(
            'Changing user password failed: pexpect session timed out')
    if child.isalive():
        raise JujuAssertionError(
            'Changing user password failed: pexpect session still alive')
    child.close()
    if child.exitstatus != 0:
        raise JujuAssertionError(
            'Changing user password failed: '
            'pexpect process exited with {}'.format(child.exitstatus))


def assert_disable_enable(controller_client, user):
    """Test disabling and enabling users"""
    controller_client.disable_user(user.name)
    user_list = list_users(controller_client)
    if sorted(user_list) != sorted(user_list_admin_read):
        raise JujuAssertionError(user_list)
    controller_client.enable_user(user.name)
    user_list = list_users(controller_client)
    if sorted(user_list) != sorted(user_list_admin_read_write):
        raise JujuAssertionError(user_list)


def assert_logout_login(controller_client, user_client, user, fake_home):
    """Test users' login and logout"""
    user_client.logout()
    user_list = list_users(controller_client)
    if sorted(user_list) != sorted(user_list_admin_read):
        raise JujuAssertionError(user_list)
    username = user.name
    controller_name = '{}_controller'.format(username)
    client = controller_client.create_cloned_environment(
        fake_home, controller_name, user.name)
    try:
        child = client.expect('login', (user.name, '-c', controller_name),
                              include_e=False)
        child.expect('(?i)password')
        child.sendline(user.name + '_password_2')
        child.expect(pexpect.EOF)
        if child.isalive():
            raise JujuAssertionError(
                'Login user failed: pexpect session still alive')
        child.close()
        if child.exitstatus != 0:
            raise JujuAssertionError(
                'Login user failed: pexpect process exited with {}'.format(
                    child.exitstatus))
    except pexpect.TIMEOUT:
        raise JujuAssertionError(
            'Login user failed: pexpect session timed out')


def assert_read_user(controller_client, user):
    """Assess the operations of read user"""
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        user_list = list_users(controller_client)
        share_list = list_shares(controller_client)
        if sorted(user_list) != sorted(user_list_admin_read):
            raise JujuAssertionError(user_list)
        if sorted(share_list) != sorted(share_list_admin_read):
            raise JujuAssertionError(share_list)
        assert_change_password(user_client, user)
        assert_logout_login(controller_client, user_client, user, fake_home)
        assert_user_permissions(user, user_client, controller_client)


def assert_write_user(controller_client, user):
    """Assess the operations of write user"""
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        user_list = list_users(controller_client)
        share_list = list_shares(controller_client)
        if sorted(user_list) != sorted(user_list_admin_read_write):
            raise JujuAssertionError(user_list)
        if sorted(share_list) != sorted(share_list_admin_read_write):
            raise JujuAssertionError(share_list)
        assert_disable_enable(controller_client, user)
        assert_user_permissions(user, user_client, controller_client)
        wait_for_removed_services(user_client, 'cs:ubuntu')


def assert_admin_user(controller_client, user):
    """Assess the operations of admin user"""
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        user_list = list_users(controller_client)
        share_list = list_shares(controller_client)
        if sorted(user_list) != sorted(user_list_all):
            raise JujuAssertionError(user_list)
        if sorted(share_list) != sorted(share_list_all):
            raise JujuAssertionError(share_list)
        assert_user_permissions(user, user_client, controller_client)


def assess_user_grant_revoke(controller_client):
    """Test multi-users functionality"""
    controller_client.env.user_name = 'admin'
    log.debug("Creating Users")
    read_user = User('readuser', 'read',
                     [True, False, False, False, False, False])
    write_user = User('writeuser', 'write',
                      [True, True, False, True, False, False])
    admin_user = User('adminuser', 'admin',
                      [True, True, True, True, True, True])
    user_list = list_users(controller_client)
    share_list = list_shares(controller_client)
    user_status = show_user(controller_client)
    if sorted(user_list) != sorted(user_list_admin):
        raise JujuAssertionError(user_list)
    if share_list != share_list_admin:
        raise JujuAssertionError(share_list)
    if user_status != user_list_admin[0]:
        raise JujuAssertionError(user_status)
    assert_read_user(controller_client, read_user)
    assert_write_user(controller_client, write_user)
    assert_admin_user(controller_client, admin_user)


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
