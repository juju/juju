#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""

from __future__ import print_function

import argparse
import logging
import sys
import tempfile
import subprocess
import pexpect
import os

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    scoped_environ,
)

from jujupy import (
    make_client,
)


__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")


def assess_user_grant_revoke(client, juju_bin):
    # Deploy charms, there are several under ./repository
    #client.juju("deploy", ('local:xenial/wordpress',))
    # Wait for the deployment to finish.
    client.wait_for_started()
    log.info("Creating Users")
    bob_env = create_user_shell_env()
    carol_env = create_user_shell_env()
    model = client.get_admin_model_name()
    bob_register = get_register_command(client.get_juju_output('add-user',  'bob', '--models', ' '+ model, '--acl', ' read'))
    carol_register = get_register_command(client.get_juju_output('add-user',  'carol', '--models', ' '+ model))


    # can we pass in env to get_juju_output?
    # should we use temp_bootstrap_env?


    with scoped_environ(bob_env):
        client = make_client(juju_bin, False, bob_env)
        child = pexpect.spawnu(juju_bin + bob_register)

        child.expect('(?i)name .*: ')
        child.sendline('bob_controller')
        child.expect('(?i)password')
        child.sendline('bob')
        child.expect('(?i)password')
        child.sendline('bob')
        child.close()
        #if child.isalive():
        #    raise Exception
        client.list_models()


    with scoped_environ(carol_env):
        client = make_client(juju_bin, False, carol_env)
        child = pexpect.spawnu(juju_bin + carol_register)

        child.expect('(?i)name .*: ')
        child.sendline('carol_controller')
        child.expect('(?i)password')
        child.sendline('carol')
        child.expect('(?i)password')
        child.sendline('carol')
        child.close()
        #if child.isalive():
        #    raise Exception
        client.list_models()


    #bob_register = client.get_juju_output('add-user',  'bob', '--models', ' blog', '--acl', ' read')
    #bob_register = get_register_command(get_output('add-user bob --models blog --acl read'))
    #carol_register = get_register_command(get_output('add-user carol --models blog'))
    #subprocess.check_output(['juju', 'add-user', 'carol', '--models', ' blog'])

    #juju bootstrap lxd lxd --upload-tools
    #juju create-model blog

    #juju add-user bob --models blog --acl read
    #mkdir /tmp/bob
    #export XDG_DATA_HOME=/tmp/bob
    #juju register

def create_user_shell_env():
    env = dict(os.environ)
    env['XDG_DATA_HOME'] = tempfile.mkdtemp()
    return env

def get_register_command(output):
    #b'User "carol" added\nUser "carol" granted read access to model "blog"\nPlease send this command to carol:\n    juju register MEATBWNhcm9sMBUTEzEwLjIwOC41Ni4yNTI6MTcwNzAEIEBAY-SXp7WeoJv6FwDU8p6JXFAXi8ayZwk8qN4Ai1By\n'
    for row in output.split('\n'):
        if 'juju register' in row:
            return row.strip().replace("juju","",1)

def get_output(command, juju_path=None):
    if juju_path is None:
        juju_path = 'juju'
    return subprocess.check_output((juju_path, command)).strip()

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="TODO: script info")
    # TODO: Add additional positional arguments.
    add_basic_testing_arguments(parser)
    # TODO: Add additional optional arguments.
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    juju_bin = args.juju_bin
    with bs_manager.booted_context(args.upload_tools):
        assess_user_grant_revoke(bs_manager.client, juju_bin)
    return 0


if __name__ == '__main__':
    sys.exit(main())
