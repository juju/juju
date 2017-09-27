#!/usr/bin/env python

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
from textwrap import dedent
from subprocess import CalledProcessError
import sys

from jujucharm import (
    local_charm_path,
)
from jujupy import (
    client_from_config,
    fake_juju_client,
    SimpleEnvironment,
    )
from deploy_stack import (
    BootstrapManager,
    check_token,
    get_random_string,
    )
from jujuci import add_credential_args
from utility import (
    configure_logging,
    until_timeout,
)


def prepare_dummy_env(client):
    """Use a client to prepare a dummy environment."""
    charm_source = local_charm_path(
        charm='dummy-source', juju_ver=client.version)
    client.deploy(charm_source)
    charm_sink = local_charm_path(charm='dummy-sink', juju_ver=client.version)
    client.deploy(charm_sink)
    token = get_random_string()
    client.set_config('dummy-source', {'token': token})
    client.juju('add-relation', ('dummy-source', 'dummy-sink'))
    client.juju('expose', ('dummy-sink',))
    return token


def get_clients(initial, other, base_env, debug, agent_url):
    """Return the clients to use for testing."""
    if initial == 'FAKE':
        environment = SimpleEnvironment.from_config(base_env)
        client = fake_juju_client(env=environment)
        return client, client, client
    else:
        initial_client = client_from_config(base_env, initial, debug=debug)
        environment = initial_client.env
    if agent_url is None:
        environment.discard_option('tools-metadata-url')
    other_client = initial_client.clone_from_path(other)
    # This used to catch an exception of the config didn't match.
    # version_client no longer exists so that no longer made sense.
    released_client = initial_client.clone_from_path(None)
    # If system juju is used, ensure it has identical env to
    # initial_client.
    released_client.env = initial_client.env
    return initial_client, other_client, released_client


def assess_heterogeneous(initial, other, base_env, environment_name, log_dir,
                         upload_tools, debug, agent_url, agent_stream, series):
    """Top level function that prepares the clients and environment.

    initial and other are paths to the binary used initially, and a binary
    used later.  base_env is the name of the environment to base the
    environment on and environment_name is the new name for the environment.
    """
    initial_client, other_client, teardown_client = get_clients(
        initial, other, base_env, debug, agent_url)
    jes_enabled = initial_client.is_jes_enabled()
    bs_manager = BootstrapManager(
        environment_name, initial_client, teardown_client,
        bootstrap_host=None, machines=[], series=series, agent_url=agent_url,
        agent_stream=agent_stream, region=None, log_dir=log_dir,
        keep_env=False, permanent=jes_enabled, jes_enabled=jes_enabled)
    test_control_heterogeneous(bs_manager, other_client, upload_tools)


@contextmanager
def run_context(bs_manager, other, upload_tools):
    try:
        bs_manager.keep_env = True
        with bs_manager.booted_context(upload_tools):
            if other.env.juju_home != bs_manager.client.env.juju_home:
                raise AssertionError('Juju home out of sync')
            yield
        # Test clean shutdown of an environment.
        callback_with_fallback(other, bs_manager.tear_down_client,
                               nice_tear_down)
    except:
        bs_manager.tear_down()
        raise


def test_control_heterogeneous(bs_manager, other, upload_tools):
    """Test if one binary can control an environment set up by the other."""
    initial = bs_manager.client
    released = bs_manager.tear_down_client
    with run_context(bs_manager, other, upload_tools):
        token = prepare_dummy_env(initial)
        initial.wait_for_started()
        if sys.platform != "win32":
            # Currently, juju ssh is not working on Windows.
            check_token(initial, token)
            check_series(other)
            other.juju('run', ('--all', 'uname -a'))
        other.get_config('dummy-source')
        other.get_model_config()
        other.juju('remove-relation', ('dummy-source', 'dummy-sink'))
        status = other.get_status()
        other.juju('unexpose', ('dummy-sink',))
        status = other.get_status()
        if status.get_applications()['dummy-sink']['exposed']:
            raise AssertionError('dummy-sink is still exposed')
        status = other.get_status()
        charm_path = local_charm_path(
            charm='dummy-sink', juju_ver=other.version)
        juju_with_fallback(other, released, 'deploy',
                           (charm_path, 'sink2'))
        other.wait_for_started()
        other.juju('add-relation', ('dummy-source', 'sink2'))
        status = other.get_status()
        other.juju('expose', ('sink2',))
        status = other.get_status()
        if 'sink2' not in status.get_applications():
            raise AssertionError('Sink2 missing')
        other.remove_service('sink2')
        for ignored in until_timeout(30):
            status = other.get_status()
            if 'sink2' not in status.get_applications():
                break
        else:
            raise AssertionError('Sink2 not destroyed')
        other.juju('add-relation', ('dummy-source', 'dummy-sink'))
        status = other.get_status()
        relations = status.get_applications()['dummy-sink']['relations']
        if not relations['source'] == ['dummy-source']:
            raise AssertionError('source is not dummy-source.')
        other.juju('expose', ('dummy-sink',))
        status = other.get_status()
        if not status.get_applications()['dummy-sink']['exposed']:
            raise AssertionError('dummy-sink is not exposed')
        other.juju('add-unit', ('dummy-sink',))
        if not has_agent(other, 'dummy-sink/1'):
            raise AssertionError('dummy-sink/1 was not added.')
        other.juju('remove-unit', ('dummy-sink/1',))
        status = other.get_status()
        if has_agent(other, 'dummy-sink/1'):
            raise AssertionError('dummy-sink/1 was not removed.')
        container_type = other.preferred_container()
        other.juju('add-machine', (container_type,))
        status = other.get_status()
        container_machine, = set(k for k, v in status.agent_items() if
                                 k.endswith('/{}/0'.format(container_type)))
        container_holder = container_machine.split('/')[0]
        other.remove_machine(container_machine)
        wait_until_removed(other, container_machine)
        other.remove_machine(container_holder)
        wait_until_removed(other, container_holder)


# suppress nosetests
test_control_heterogeneous.__test__ = False


def juju_with_fallback(other, released, command, args, include_e=True):
    """Fallback to released juju when 1.18 fails.

    Get as much test coverage of 1.18 as we can, by falling back to a released
    juju for commands that we expect to fail (due to unsupported agent version
    format).
    """
    def call_juju(client):
        client.juju(command, args, include_e=include_e)
    return callback_with_fallback(other, released, call_juju)


def callback_with_fallback(other, released, callback):
    for client in [other, released]:
        try:
            callback(client)
        except CalledProcessError:
            if not client.version.startswith('1.18.'):
                raise
        else:
            break


def nice_tear_down(client):
    client.kill_controller()


def has_agent(client, agent_id):
    return bool(agent_id in dict(client.get_status().agent_items()))


def wait_until_removed(client, agent_id):
    """Wait for an agent to be removed from the environment."""
    for ignored in until_timeout(240):
        if not has_agent(client, agent_id):
            return
    else:
        raise AssertionError('Machine not destroyed: {}.'.format(agent_id))


def check_series(client,  machine='0', series=None):
    """Use 'juju ssh' to check that the deployed series meets expectations."""
    result = client.get_juju_output('ssh', machine, 'lsb_release', '-c')
    label, codename = result.rstrip().split('\t')
    if label != 'Codename:':
        raise AssertionError()
    if series:
        expected_codename = series
    else:
        expected_codename = client.env.get_option('default-series')
    if codename != expected_codename:
        raise AssertionError(
            'Series is {}, not {}'.format(codename, expected_codename))


def parse_args(argv=None):
    parser = ArgumentParser(description=dedent("""\
        Determine whether one juju version can control an environment created
        by another version.
    """))
    parser.add_argument('initial', help='The initial juju binary.')
    parser.add_argument('other', help='A different juju binary.')
    parser.add_argument('base_environment', help='The environment to base on.')
    parser.add_argument('environment_name', help='The new environment name.')
    parser.add_argument('log_dir', help='The directory to dump logs to.')
    parser.add_argument(
        '--upload-tools', action='store_true', default=False,
        help='Upload local version of tools before bootstrapping.')
    parser.add_argument('--debug', help='Run juju with --debug',
                        action='store_true', default=False)
    parser.add_argument('--agent-url', default=None)
    parser.add_argument('--agent-stream', action='store',
                        help='URL for retrieving agent binaries.')
    parser.add_argument('--series', action='store',
                        help='Name of the Ubuntu series to use.')
    add_credential_args(parser)
    return parser.parse_args(argv)


def main():
    args = parse_args()
    configure_logging(logging.INFO)
    assess_heterogeneous(args.initial, args.other, args.base_environment,
                         args.environment_name, args.log_dir,
                         args.upload_tools, args.debug, args.agent_url,
                         args.agent_stream, args.series)


if __name__ == '__main__':
    main()
