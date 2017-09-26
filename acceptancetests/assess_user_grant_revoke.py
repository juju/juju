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
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
    )

__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")

User = namedtuple('User', ['name', 'permissions', 'expect'])


USER_LIST_CTRL = [{"access": "superuser", "user-name": "admin",
                   "display-name": "admin"}]
USER_LIST_CTRL_READ = copy.deepcopy(USER_LIST_CTRL)
# Created user has no display name, bug 1606354
USER_LIST_CTRL_READ.append(
    {"access": "login", "user-name": "readuser"})
USER_LIST_CTRL_WRITE = copy.deepcopy(USER_LIST_CTRL)
# bug 1606354
USER_LIST_CTRL_WRITE.append({"access": "login", "user-name": "writeuser"})
USER_LIST_CTRL_ADMIN = copy.deepcopy(USER_LIST_CTRL)
# bug 1606354
USER_LIST_CTRL_ADMIN.append(
    {"access": "superuser", "user-name": "adminuser"})
SHARE_LIST_CTRL = {"admin": {"display-name": "admin",
                             "access": "admin"}}
SHARE_LIST_CTRL_READ = copy.deepcopy(SHARE_LIST_CTRL)
SHARE_LIST_CTRL_READ["readuser"] = {"access": "read"}
SHARE_LIST_CTRL_WRITE = copy.deepcopy(SHARE_LIST_CTRL)
SHARE_LIST_CTRL_WRITE["writeuser"] = {"access": "write"}
SHARE_LIST_CTRL_ADMIN = copy.deepcopy(SHARE_LIST_CTRL)
SHARE_LIST_CTRL_ADMIN["adminuser"] = {"access": "admin"}


def _generate_random_string():
    # We prefix a letter because usernames must begin with letter
    return 'r'.join(random.choice(
        string.ascii_letters + string.digits) for _ in range(9))


def assert_equal(found, expected):
    found = sorted(found)
    expected = sorted(expected)
    if found != expected:
        raise JujuAssertionError(
            'Found: {}\nExpected: {}'.format(found, expected))


def assert_command_fails(check_callable, command_type, permission):
    try:
        check_callable()
    except subprocess.CalledProcessError:
        pass
    else:
        raise JujuAssertionError(
            'FAIL User performed {} operation with '
            'permission {}'.format(command_type, permission))


def assert_command_succeeds(check_callable, command_type, permission):
    try:
        check_callable()
    except subprocess.CalledProcessError:
        raise JujuAssertionError(
            'FAIL User unable to perform {} operation with '
            'permission {}'.format(command_type, permission))


def list_users(client):
    """Test listing all the users"""
    users_list = json.loads(client.get_juju_output('list-users', '--format',
                                                   'json', include_e=False))
    for user in users_list:
        user.pop("date-created", None)
        user.pop("last-connection", None)
    return users_list


def list_shares(client):
    """Test listing users' shares"""
    model_data = json.loads(
        client.get_juju_output(
            'show-model', '--format', 'json', include_e=False))
    share_list = model_data[client.model_name]['users']
    for key, value in share_list.iteritems():
        value.pop("last-connection", None)
    return share_list


def show_user(client):
    """Test showing a user's status"""
    user_status = json.loads(client.get_juju_output('show-user', '--format',
                                                    'json', include_e=False))
    user_status.pop("date-created", None)
    user_status.pop("last-connection", None)
    return user_status


def assess_read_operations(client, permission, has_permission):
    read_commands = (
        client.show_status,
        lambda: client.juju('show-model', (), include_e=False),
    )

    for command in read_commands:
        if has_permission:
            assert_command_succeeds(command, 'read', permission)
        else:
            assert_command_fails(command, 'read', permission)


def assess_write_operations(client, permission, has_permission):
    tags = '"{}={}"'.format(client.env.user_name, permission)
    write_commands = (
        lambda: client.set_env_option('resource-tags', tags),
    )

    for command in write_commands:
        if has_permission:
            assert_command_succeeds(command, 'write', permission)
        else:
            assert_command_fails(command, 'write', permission)


def assess_admin_operations(client, permission, has_permission):
    # Create a username for the user client to interact with
    new_read_user = _generate_random_string()
    new_admin_user = _generate_random_string()
    admin_commands = (
        lambda: client.add_user(new_read_user),
        lambda: client.grant(new_read_user, permission="read"),
        lambda: client.remove_user(new_read_user),
        lambda: client.add_user_perms(new_admin_user, permissions="admin"),
        lambda: client.remove_user(new_admin_user),
    )

    for command in admin_commands:
        if has_permission:
            assert_command_succeeds(command, 'admin', permission)
        else:
            assert_command_fails(command, 'admin', permission)


def assert_read_model(client, permission, has_permission):
    """Test if the user has or doesn't have the read permission"""
    log.info('Checking read model acl {}'.format(client.env.user_name))
    assess_read_operations(client, permission, has_permission)
    log.info('PASS {} read acl'.format(client.env.user_name))


def assert_write_model(client, permission, has_permission):
    """Test if the user has or doesn't have the write permission"""
    log.info('Checking write model acl {}'.format(client.env.user_name))
    assess_write_operations(client, permission, has_permission)
    log.info('PASS {} write model acl'.format(client.env.user_name))


def assert_admin_model(controller_client, client, permission, has_permission):
    """Test if the user has or doesn't have the admin permission"""
    log.info('Checking admin acl with {}'.format(client.env.user_name))
    assess_admin_operations(client, permission, has_permission)
    log.info('PASS {} admin acl'.format(client.env.user_name))


def assert_user_permissions(user, user_client, controller_client):
    """Test if users' permissions are within expectations"""
    expect = iter(user.expect)
    permission = user.permissions
    assert_read_model(user_client, permission, expect.next())
    assert_write_model(user_client, permission, expect.next())
    assert_admin_model(
        controller_client, user_client, permission, expect.next())

    log.info("Revoking {} permission from {}".format(
        user.permissions, user.name))
    controller_client.revoke(user.name, permissions=user.permissions)
    log.info('Revoke accepted')

    assert_read_model(user_client, permission, expect.next())
    assert_write_model(user_client, permission, expect.next())
    assert_admin_model(
        controller_client, user_client, permission, expect.next())


def assert_change_password(client, user, password):
    """Test changing user's password"""
    log.info('Checking change-user-password')
    try:
        child = client.expect('change-user-password', (user.name,),
                              include_e=False)
        child.expect('(?i)password')
        child.sendline(password)
        child.expect('(?i)password')
        child.sendline(password)
        client._end_pexpect_session(child)
    except pexpect.TIMEOUT:
        log.error('Buffer: {}'.format(child.buffer))
        log.error('Before: {}'.format(child.before))
        raise JujuAssertionError(
            'FAIL Changing user password failed: '
            'pexpect process exited with {}'.format(child.exitstatus))
    log.info('PASS change-user-password')


def assert_disable_enable(controller_client, user):
    """Test disabling and enabling users"""
    original_user_list = list_users(controller_client)
    log.info('Checking disabled {}'.format(user.name))
    controller_client.disable_user(user.name)
    log.info('Disabled {}'.format(user.name))
    user_list = list_users(controller_client)
    log.info('Checking list-users {}'.format(user.name))
    assert_equal(user_list, USER_LIST_CTRL)
    log.info('Checking enable {}'.format(user.name))
    controller_client.enable_user(user.name)
    log.info('Enabled {}'.format(user.name))
    user_list = list_users(controller_client)
    log.info('Checking list-users {}'.format(user.name))
    assert_equal(user_list, original_user_list)


def assert_user_status(client, user, expected_users, expected_shares):
    """Test listing users and shares against expected values"""
    log.info('Checking list-users {}'.format(user.name))
    user_list = list_users(client)
    assert_equal(user_list, expected_users)
    log.info('Checking list-shares {}'.format(user.name))
    share_list = list_shares(client)
    assert_equal(share_list, expected_shares)


def assert_logout_login(controller_client, user_client, user,
                        fake_home, password, expected_users):
    """Test users' login and logout"""
    original_user_list = list_users(controller_client)
    user_client.logout()
    log.info('Checking list-users after logout')
    user_list = list_users(controller_client)
    assert_equal(user_list, expected_users)
    log.info('Checking list-users after login')
    user_client.login_user(user.name, password)
    user_list = list_users(controller_client)
    assert_equal(user_list, original_user_list)


def assert_read_user(controller_client, user):
    """Assess the operations of read user"""
    log.info('Checking read {}'.format(user.name))
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        user_client.env.user_name = user.name
        assert_user_status(controller_client, user,
                           USER_LIST_CTRL_READ, SHARE_LIST_CTRL_READ)

        password = _generate_random_string()
        assert_change_password(user_client, user, password)
        assert_logout_login(controller_client, user_client,
                            user, fake_home, password, USER_LIST_CTRL_READ)
        assert_user_permissions(user, user_client, controller_client)
        assert_disable_enable(controller_client, user)
        controller_client.remove_user(user.name)
    log.info('PASS read {}'.format(user.name))


def assert_write_user(controller_client, user):
    """Assess the operations of write user"""
    log.info('Checking write {}'.format(user.name))
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        user_client.env.user_name = user.name
        assert_user_status(controller_client, user,
                           USER_LIST_CTRL_WRITE, SHARE_LIST_CTRL_WRITE)

        password = _generate_random_string()
        assert_change_password(user_client, user, password)
        assert_logout_login(controller_client, user_client,
                            user, fake_home, password, USER_LIST_CTRL_WRITE)
        assert_user_permissions(user, user_client, controller_client)
        assert_disable_enable(controller_client, user)
        controller_client.remove_user(user.name)
    log.info('PASS write {}'.format(user.name))


def assert_admin_user(controller_client, user):
    """Assess the operations of admin user"""
    log.info('Checking admin {}'.format(user.name))
    with temp_dir() as fake_home:
        user_client = controller_client.register_user(
            user, fake_home)
        controller_client.grant(user_name=user.name, permission="superuser")
        user_client.env.user_name = user.name
        assert_user_status(controller_client, user,
                           USER_LIST_CTRL_ADMIN, SHARE_LIST_CTRL_ADMIN)

        password = _generate_random_string()
        assert_change_password(user_client, user, password)
        assert_logout_login(controller_client, user_client,
                            user, fake_home, password, USER_LIST_CTRL_ADMIN)
        assert_user_permissions(user, user_client, controller_client)
        assert_disable_enable(controller_client, user)
        controller_client.remove_user(user.name)
    log.info('PASS admin {}'.format(user.name))


def assert_controller(controller_client):
    log.info('Checking list-users admin')
    user_list = list_users(controller_client)
    assert_equal(user_list, USER_LIST_CTRL)

    log.info('Checking list-shares admin')
    share_list = list_shares(controller_client)
    assert_equal(share_list, SHARE_LIST_CTRL)

    log.info('Checking show-user admin')
    user_status = show_user(controller_client)
    assert_equal(user_status, USER_LIST_CTRL[0])


def assess_user_grant_revoke(controller_client):
    """Test multi-users functionality"""
    log.info('STARTING grant/revoke permissions')
    controller_client.env.user_name = 'admin'
    log.info("Creating Users: readuser, writeuser, adminuser")
    read_user = User('readuser', 'read',
                     [True, False, False, False, False, False])
    write_user = User('writeuser', 'write',
                      [True, True, False, True, False, False])
    admin_user = User('adminuser', 'admin',
                      [True, True, True, True, True, True])

    # check controller client
    assert_controller(controller_client)

    # check each type of user
    assert_read_user(controller_client, read_user)
    assert_write_user(controller_client, write_user)
    assert_admin_user(controller_client, admin_user)

    log.info('SUCCESS grant/revoke permissions')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test grant and revoke permissions for users")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_user_grant_revoke(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
