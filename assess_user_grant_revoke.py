#!/usr/bin/env python
"""TODO: add rough description of what is assessed in this module."""
from __future__ import print_function

import argparse
import logging
import sys
import tempfile
import subprocess
import os

# seperate third party lib
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

from jujupy import (
    make_client,
    SimpleEnvironment,
    EnvJujuClient,
    JujuData,
    )

from tests import (
    use_context,
    )


__metaclass__ = type


log = logging.getLogger("assess_user_grant_revoke")


def assess_user_grant_revoke(client, juju_bin):
    # Deploy charms, there are several under ./repository
    # set JUJU_REPOSITORY, this will work
    #client.juju("deploy", ('local:xenial/wordpress',))
    # Wait for the deployment to finish.
    client.wait_for_started()
    #model = client.get_admin_model_name()
    #model = client.create_model('user-access')
    model = client.env.environment

    logging.debug("Creating Users")
    try:
        adduser = client.get_juju_output('add-user',  'bob', '--models', model, '--acl', 'read', include_e=False)
        bob_register = get_register_command(adduser)
    except subprocess.CalledProcessError as e:
        logging.warn(e)
        logging.warn(e.stderr)

    try:
        adduser = client.get_juju_output('add-user',  'carol', '--models', model, include_e=False)
        carol_register = get_register_command(adduser)
    except subprocess.CalledProcessError as e:
        logging.warn(e)
        logging.warn(e.stderr)

    #import pdb
    #pdb.set_trace()

    # can we pass in env to get_juju_output?
    # should we use temp_bootstrap_env?

    # make it a context manager, because of cleanup
    # make finally block with .close() and .isalive()
    # move to jujupy
    # with client.pexpect session:

    # use fake juju, look at lp:~abentley/juju-ci-tools/keystone3

    # keep pexpect in here, but factor it out
    # don't merge into jujupy until we need to do more

    logging.debug("Testing Bob access")
    #bob_env = _shell_environ
    #log.debug(bob_env)
    #with temp_bootstrap_env(fake_home, client):
    #bob_env = create_user_shell_env()

    #bob_env = create_user_shell_env()
    #with scoped_environ(bob_env):
    #bob_env = SimpleEnvironment('bob', {'type': 'local'})
    #fake_home = use_context(self, temp_dir())
    pdb.set_trace()
    with temp_dir() as fake_home:
        #bob_env = JujuData('admin', juju_home=fake_home)
        #bob_client = EnvJujuClient(bob_env, '2.0-fake', juju_bin)

        # juju login

        bob_client = client.clone(env=client.env.clone())
        bob_client.env.juju_home = fake_home

        bob_shell_env = bob_client._shell_environ()

        # needs support to passing register command with arguments
        # refactor once supported, bug 1573099
        with scoped_environ(bob_shell_env):
            child = pexpect.spawn(juju_bin + bob_register)
        child.expect('(?i)name .*: ')
        child.sendline('bob_controller')
        child.expect('(?i)password')
        child.sendline('bob')
        child.expect('(?i)password')
        child.sendline('bob')
        child.close()
            #if child.isalive():
            #    raise Exception


        log.debug('Bob controller')
        log.debug(bob_client.get_juju_output('show-controller', include_e=False))

        # we SHOULD NOT be able to deploy
        try:
            log.debug(bob_client.show_status())
            bob_client.deploy('wordpress')
            #log.debug('assert_fail read-only user deployed charm')
            raise AssertionError('assert_fail read-only user deployed charm')
        except subprocess.CalledProcessError as e:
            log.debug('bob could not deploy')
            pass


        # remove permissions from bob
        logging.debug("Revoking permissions from bob")
        try:
            adduser = client.get_juju_output('revoke',  'bob', model, include_e=False)
            bob_register = get_register_command(adduser)
        except subprocess.CalledProcessError as e:
            logging.warn(e)
            logging.warn(e.stderr)

        # Bob should see nothing
        # we SHOULD NOT be able to do anything
        logging.debug("Testing Bob access")
        try:
            log.debug(bob_client.list_models())
            #log.debug('assert_fail revoked user sees models')
            raise AssertionError('assert_fail read-only user deployed charm')
        except subprocess.CalledProcessError:
            pass


    ######################3
    logging.debug("Testing Carol access")
    carol_env = SimpleEnvironment('carol', {'type': 'local'})
    carol_client = EnvJujuClient(carol_env, '2.0-fake', juju_bin, create_user_shell_env())
    # needs support to passing register command with arguments
    # refactor once supported, bug 1573099
    child = pexpect.spawn(juju_bin + carol_register)
    child.expect('(?i)name .*: ')
    child.sendline('carol_controller')
    child.expect('(?i)password')
    child.sendline('carol')
    child.expect('(?i)password')
    child.sendline('carol')
    child.close()
    #if child.isalive():
    #    raise Exception

    #log.debug('Carol controller')
    #log.debug(carol_client.get_juju_output('show-controller', include_e=False))


    # we SHOULD NOT be able to deploy
    try:
        carol_client.deploy('wordpress')
        log.debug('carol could deploy')
    except subprocess.CalledProcessError:
        raise AssertionError('assert_fail admin user could not deploy charm')

    # remove permissions from bob and carol
    logging.debug("Revoking permissions")
    try:
        adduser = client.get_juju_output('revoke',  'bob', model, include_e=False)
        bob_register = get_register_command(adduser)
    except subprocess.CalledProcessError as e:
        logging.warn(e)
        logging.warn(e.stderr)

    try:
        adduser = client.get_juju_output('revoke',  'carol', '--acl', 'write', model, include_e=False)
        bob_register = get_register_command(adduser)
    except subprocess.CalledProcessError as e:
        logging.warn(e)
        logging.warn(e.stderr)


    # Carol should be able to get status
    # Bob should see nothing
    # we SHOULD NOT be able to do anything
    logging.debug("Testing Bob access")
    try:
        log.debug(bob_client.list_models())
        log.debug('assert_fail revoked user sees models')
        #raise AssertionError('assert_fail read-only user deployed charm')
    except subprocess.CalledProcessError:
        pass


    logging.debug("Testing Carol access")
    # we SHOULD be able to see models
    try:
        log.debug(carol_client.list_models())
    except subprocess.CalledProcessError:
        log.debug('assert_fail read-only user deployed charm')
        #raise AssertionError('assert_fail read-only user deployed charm')

    # we SHOULD NOT be able to deploy
    try:
        carol_client.deploy('mysql')
        log.debug('assert_fail read-only user deployed charm')
        #raise AssertionError('assert_fail read-only user deployed charm')
    except subprocess.CalledProcessError:
        pass


# use _shell_environ
def create_user_shell_env():
    env = dict(os.environ)
    env['XDG_DATA_HOME'] = tempfile.mkdtemp()
    return env

def get_register_command(output):
    #b'User "carol" added\nUser "carol" granted read access to model "blog"\nPlease send this command to carol:\n    juju register MEATBWNhcm9sMBUTEzEwLjIwOC41Ni4yNTI6MTcwNzAEIEBAY-SXp7WeoJv6FwDU8p6JXFAXi8ayZwk8qN4Ai1By\n'

    # use re here instead
    # use regex
    #log.debug(jim_r)
   # " register MDoTA2ppbTAREw8xMC4wLjMuMzk6MTcwNzAEII7nwGOIWoRZZ1pXI2I_dKjGhpM-Ja5If_BndAcaJuQt"
    for row in output.split('\n'):
        if 'juju register' in row:
            return row.strip().replace("juju","",1)

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="TODO: script info")
    # TODO: Add additional positional arguments.
    add_basic_testing_arguments(parser)
    # TODO: Add additional optional arguments.
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
    import pdb, traceback, sys
    try:
        main()
    except:
        type, value, tb = sys.exc_info()
        traceback.print_exc()
        pdb.post_mortem(tb)
    #sys.exit(main())
