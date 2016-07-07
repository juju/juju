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

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    scoped_environ,
    temp_dir,
)

__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")


user_list_1 = [{"user-name":"admin","display-name":"admin"}]
user_list_2 = copy.deepcopy(user_list_1)
user_list_2.append({"user-name":"readuser","display-name":""})
user_list_3 = copy.deepcopy(user_list_2)
user_list_3.append({"user-name":"adminuser","display-name":""})
user_list_3.remove({"user-name":"readuser","display-name":""})
share_list_1 = {"admin@local": {"display-name": "admin", "access": "admin"}}
share_list_2 = copy.deepcopy(share_list_1)
share_list_2["readuser@local"] = {"access": "read"}
share_list_3 = copy.deepcopy(share_list_2)
share_list_3["adminuser@local"] = {"access": "write"}
share_list_3.pop("readuser@local")



# This needs refactored out to utility
class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


def list_users(client):
    users_list = json.loads(client.get_juju_output('list-users', '--format', 'json', include_e=False))
    try:
        for user in users_list:
            user.pop("date-created")
            user.pop("last-connection")
    except Exception:
        pass
    return users_list


def list_shares(client):
    share_list = json.loads(client.get_juju_output('list-shares', '--format', 'json', include_e=False))
    try:
        for key, value in share_list.iteritems():
            value.pop("last-connection")
    except Exception:
        pass
    return share_list


def show_user(client):
    user_status = json.loads(client.get_juju_output('show-user', '--format', 'json', include_e=False))
    try:
        user_status.pop("date-created")
        user_status.pop("last-connection")
    except Exception:
        pass
    return user_status


def register_user(user, client, fake_home):
    """Register `user` for the `client` return the cloned client used."""
    # needs support to passing register command with arguments
    # refactor once supported, bug 1573099
    # pexpect has a bug, and doesn't honor env=
    username = user.name
    try:
        token = client._backend.add_user(username, permission=user.permissions)
    except Exception:
        token = client.add_user(username, permissions=user.permissions)
        pass
    user_client, user_env = create_cloned_environment(
        client, fake_home)
    try:
        with scoped_environ(user_env):
            try:
                child = user_client.expect('register', (token,), include_e=False)
                child.expect('(?i)name')
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
    except Exception:
        pass
    return user_client


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


def assert_users_shares(client, user):
    if user.name == 'readuser':
        user_list_expected = user_list_2
        share_list_expected = share_list_2
    else:
        user_list_expected = user_list_3
        share_list_expected = share_list_3
    user_list = list_users(client)
    share_list = list_shares(client)
    if sorted(user_list) != sorted(user_list_expected):
        raise JujuAssertionError(user_list)
    if sorted(share_list) != sorted(share_list_expected):
        raise JujuAssertionError(share_list)


def assess_change_password(client, user, fake_home):
    user_client, user_env = create_cloned_environment(
        client, fake_home)
    with scoped_environ(user_env):
        try:
            child = user_client.expect('change-user-password', (user.name,), include_e=False)
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


def assess_disable_enable(admin_client, users):
    admin_client.disable_user(users[-1].name)
    user_list = list_users(admin_client)
    if sorted(user_list) != sorted(user_list_1):
        raise JujuAssertionError(user_list)
    # admin_client.disable_user(users[-2].name)
    # user_list = list_users(admin_client)
    # if sorted(user_list) != sorted(user_list_1):
    #     raise JujuAssertionError(user_list)
    # admin_client.enable_user(users[-2].name)
    # user_list = list_users(admin_client)
    # if sorted(user_list) != sorted(user_list_2):
    #     raise JujuAssertionError(user_list)
    admin_client.enable_user(users[-1].name)
    user_list = list_users(admin_client)
    if sorted(user_list) != sorted(user_list_3):
        raise JujuAssertionError(user_list)


def assess_logout_login(admin_client, user_client, user, fake_home):
    user_client.logout_user()
    user_list = list_users(admin_client)
    if sorted(user_list) != sorted(user_list_3):
        raise JujuAssertionError(user_list)
    client, user_env = create_cloned_environment(
        admin_client, fake_home)
    with scoped_environ(user_env):
        try:
            child = client.expect('login', (user.name,), include_e=False)
            child.expect('(?i)password')
            child.sendline(user.name + '_password_2')
            child.expect(pexpect.EOF)
            if child.isalive():
                raise JujuAssertionError(
                    'Login user failed: pexpect session still alive')
        except pexpect.TIMEOUT:
            raise JujuAssertionError(
                'Login user failed: pexpect session timed out')


def assess_user_grant_revoke(admin_client):
    log.debug("Creating Users")
    user = namedtuple('user', ['name', 'permissions', 'expect'])
    read_user = user('readuser', 'read', [True, False, False, False])
    write_user = user('adminuser', 'write', [True, True, True, False])
    users = [read_user, write_user]
    user_list = list_users(admin_client)
    share_list = list_shares(admin_client)
    user_status = show_user(admin_client)
    if sorted(user_list) != sorted(user_list_1):
        raise JujuAssertionError(user_list)
    if share_list != share_list_1:
        raise JujuAssertionError(share_list)
    if user_status != user_list_1[0]:
        raise JujuAssertionError(user_status)
    for user in users:
        log.debug("Testing %s" % user.name)
        with temp_dir() as fake_home:
            user_client = register_user(
                user, admin_client, fake_home)
            assert_users_shares(user_client, user)
            assess_change_password(user_client, user, fake_home)
            if user.name == 'adminuser':
                assess_disable_enable(admin_client, users)
                assess_logout_login(admin_client, user_client, user, fake_home)
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
