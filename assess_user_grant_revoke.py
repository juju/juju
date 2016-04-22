#!/usr/bin/env python
"""This testsuite is intended to test basic user permissions. Users
   can be granted read or full priveleges by model. Revoking those
   priveleges should remove them."""
   
from __future__ import print_function

import argparse
import logging
import sys
import tempfile
import subprocess
import os

from deploy_stack import (
    BootstrapManager,
    )

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    scoped_environ,
    temp_dir,
    )

from jujupy import (
    make_client,
    SimpleEnvironment,
    EnvJujuClient,
    JujuData,
    )

from tests import (
    use_context,
    )

import pexpect

__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")

def _get_register_command(output):
    for row in output.split('\n'):
        if 'juju register' in row:
            return row.strip().replace("juju","",1)

def create_user_permissions(client, user_name, models=None, permissions='read'):
    if models == None:
        models = client.env.environment
    
    try:
        output = client.get_juju_output('add-user', user_name, '--models', models, '--acl', permissions, include_e=False)
        return _get_register_command(output)
    except subprocess.CalledProcessError as e:
        log.warn(e)
        log.warn(e.stderr)

def remove_user_permissions(client, user_name, models=None, permissions='read'):
    if models == None:
        models = client.env.environment
    
    try:
        output = client.get_juju_output('revoke', user_name, models, '--acl', permissions, include_e=False)
    except subprocess.CalledProcessError as e:
        log.warn(e)
        log.warn(e.stderr)

def register_user(user_name, env, juju_bin):
    # needs support to passing register command with arguments
    # refactor once supported, bug 1573099
    with scoped_environ(env):
        child = pexpect.spawn(juju_bin + register_cmd)
    try:
        child.expect('(?i)user_name .*: ')
        child.sendline(user_name + '_controller')
        child.expect('(?i)password')
        child.sendline(user_name + '_password')
        child.expect('(?i)password')
        child.sendline(user_name + '_password')
        child.close()
        if child.isalive():
            raise AssertionError('Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
        raise AssertionError('Registering user failed: pexpect session timed out')

def create_cloned_environment(user_name, cloned_juju_home):
    user_client = client.clone(env=client.env.clone())
    user_client.env.juju_home = cloned_juju_home
    user_client_env = user_client._shell_environ()
    return user_client, user_client_env

def assess_user_grant_revoke(client, juju_bin):
    # Wait for the deployment to finish.
    client.wait_for_started()

    log.debug("Creating Users")
    read_user = 'read_user'
    write_user = 'write_user'
    create_user(client, read_user)

    log.debug("Testing read_user access")
    with temp_dir() as fake_home:
        read_user_client, read_user_env = create_cloned_environment(read_user, fake_home)
        register_user(read_user, read_user_env, juju_bin)

        # assert we are read_user
        # assert we are on recontroller
        
        #read_user_client.get_juju_output('show-controller', include_e=False)

        # assert we can show status
        try:
            read_user_client.show_status()
        except subprocess.CalledProcessError:
            raise AssertionError('assert_fail read-only user cannot see status')
           
        # assert we CAN NOT deploy
        try:
            read_user_client.deploy('wordpress')
            raise AssertionError('assert_fail read-only user deployed charm')
        except subprocess.CalProcessError:
            pass

        # remove all permissions
        log.debug("Revoking permissions from read_user")
        remove_user_permissions(client, read_user)

        # we SHOULD NOT be able to do anything
        log.debug("Testing read_user access")
        try:
            read_user_client.list_models()
            raise AssertionError('assert_fail zero permissions user can see status')
        except subprocess.CalledProcessError:
            pass

    # add regression check for bug 1570594

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test grant and revoke permissions for users")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(logging.DEBUG)
    bs_manager = BootstrapManager.from_args(args)
    juju_bin = args.juju_bin
    with bs_manager.booted_context(args.upload_tools):
        assess_user_grant_revoke(bs_manager.client, juju_bin)
    return 0

if __name__ == '__main__':
    sys.exit(main())
