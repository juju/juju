#!/usr/bin/env python

from contextlib import contextmanager
from time import sleep

from deploy_stack import (
    boot_context,
    check_token,
    configure_logging,
    deploy_dummy_stack,
    get_random_string,
    )
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )


def test_jes_deploy(client, charm_prefix, base_env):
    """Deploy the dummy stack in two hosted environments."""

    # deploy into state server
    deploy_dummy_stack(client, charm_prefix)

    # deploy into hosted envs
    deploy_dummy_stack_in_environ(client, charm_prefix, "env1")
    deploy_dummy_stack_in_environ(client, charm_prefix, "env2")

    # check all the services can talk
    check_services(client, base_env)
    check_services(client, "env1")
    check_services(client, "env2")


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
        SimpleEnvironment.from_config(base_env),
        EnvJujuClient.get_full_path(),
        args.debug,
    )
    with boot_context(
            args.job_name,
            client,
            args.bootstrap_host,
            args.machine,
            args.series,
            args.agent_stream,
            args.agent_url,
            args.logs, args.keep_env,
            args.upload_tools,
            args.juju_home,
            ):
        if args.machines is not None:
            client.add_ssh_machines(args.machines)
        yield client, charm_prefix, base_env


def env_token(env_name):
    return env_name + get_random_string()


def deploy_dummy_stack_in_environ(client, charm_prefix, env_name):
    env = client._shell_environ()
    # first create the environment
    client.juju(
        "system create-environment", (env_name,),
        extra_env=env,
    )

    # switch to environment
    client.env.environment = env_name
    client.juju(
        "environment set",
        ("default-series=trusty", "-e", env_name),
        extra_env=env,
    )

    # then deploy a dummy stack in it
    return deploy_dummy_stack(client, charm_prefix)


def check_updated_token(client, token, timeout):
    wait = 5
    try:
        check_token(client, token)
    except ValueError as err:
        print("INFO: waiting for token to update: {}".format(str(err)))
        if timeout > 0:
            sleep(wait)
            check_updated_token(client, token, timeout - wait)


def check_services(client, env):
    token = env_token(env)
    client.env.environment = env
    client.juju('set', ('dummy-source', 'token=%s' % token))
    print("checking services in "+env)
    check_updated_token(client, token, 30)


def main():
    parser = ArgumentParser()
    add_basic_testing_arguments(parser)
    args = parser.parse_args()
    with jes_setup(args) as (client, charm_prefix, base_env):
        test_jes_deploy(client, charm_prefix, base_env)


if __name__ == '__main__':
    main()
