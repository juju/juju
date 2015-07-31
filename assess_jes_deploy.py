#!/usr/bin/env python

from argparse import ArgumentParser
from contextlib import contextmanager
from tempfile import NamedTemporaryFile
from time import sleep

import yaml

from deploy_stack import (
    boot_context,
    check_token,
    configure_logging,
    deploy_dummy_stack,
    get_random_string,
    )
from jujupy import (
    dump_environments_yaml,
    EnvJujuClient,
    make_safe_config,
    SimpleEnvironment,
    )
from utility import add_basic_testing_arguments


def make_system_env_client(client, suffix):
    env_name = '{}-{}'.format(client.env.environment, suffix)
    system_environment = SimpleEnvironment(env_name, dict(client.env.config))
    system_env_client = EnvJujuClient.by_version(
        system_environment, client.full_path, client.debug,
    )
    system_env_client.juju_home = client.juju_home
    system_env_client.enable_jes()
    return system_env_client


def test_jes_deploy(client, charm_prefix, base_env):
    """Deploy the dummy stack in two hosted environments."""

    # deploy into state server
    deploy_dummy_stack(client, charm_prefix)

    # deploy into hosted envs
    env1_client = deploy_dummy_stack_in_environ(client, charm_prefix, "env1")
    env2_client = deploy_dummy_stack_in_environ(client, charm_prefix, "env2")

    # check all the services can talk
    check_services(client)
    check_services(env1_client)
    check_services(env2_client)


@contextmanager
def jes_setup(args):
    """jes_setup sets up the juju client and its environment.
       It returns return client, charm_prefix and base_env"""
    base_env = args.env
    configure_logging(args.verbose)
    series = args.series
    if series is None:
        series = 'precise'
    charm_prefix = 'local:{}/'.format(series)
    client = EnvJujuClient.by_version(
        SimpleEnvironment.from_config(base_env), args.juju_bin, args.debug,
    )
    client.enable_jes()
    with boot_context(
            args.temp_env_name,
            client,
            args.bootstrap_host,
            args.machine,
            args.series,
            args.agent_url,
            args.agent_stream,
            args.logs, args.keep_env,
            args.upload_tools,
            permanent=True,
            ):
        if args.machine is not None:
            client.add_ssh_machines(args.machine)
        yield client, charm_prefix, base_env


def env_token(env_name):
    return env_name + get_random_string()


def create_environment(system_client, suffix):
    client = make_system_env_client(system_client, suffix)
    with NamedTemporaryFile() as config_file:
        config = make_safe_config(client)
        yaml.dump(config, config_file)
        config_file.flush()
        client.juju(
            "system create-environment", (
                '-s', system_client.env.environment, client.env.environment,
                '--config', config_file.name),
            include_e=False)
    return client


def deploy_dummy_stack_in_environ(system_client, charm_prefix, suffix):
    # first create the environment
    client = create_environment(system_client, suffix)
    client.juju("environment set", ("default-series=trusty",))

    # then deploy a dummy stack in it
    deploy_dummy_stack(client, charm_prefix)
    return client


def check_updated_token(client, token, timeout):
    wait = 5
    try:
        check_token(client, token)
    except ValueError as err:
        print("INFO: waiting for token to update: {}".format(str(err)))
        if timeout > 0:
            sleep(wait)
            check_updated_token(client, token, timeout - wait)


def check_services(client):
    token = env_token(client.env.environment)
    client.juju('set', ('dummy-source', 'token=%s' % token))
    print("checking services in "+ client.env.environment)
    check_updated_token(client, token, 30)


def main():
    parser = ArgumentParser()
    add_basic_testing_arguments(parser)
    args = parser.parse_args()
    with jes_setup(args) as (client, charm_prefix, base_env):
        test_jes_deploy(client, charm_prefix, base_env)


if __name__ == '__main__':
    main()
