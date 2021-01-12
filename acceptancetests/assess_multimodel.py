#!/usr/bin/env python3
"""Assess multimodel support."""

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import sys
import os

from deploy_stack import (
    boot_context,
    check_token,
    configure_logging,
    deploy_dummy_stack,
    dump_env_logs,
    get_random_string,
    safe_print_status,
    )
from jujupy import (
    client_from_config,
    )
from utility import (
    add_basic_testing_arguments,
    ensure_dir,
    print_now,
    )


def assess_multimodel_deploy(client, charm_series, log_dir, base_env):
    """Deploy the dummy stack in two hosted environments."""
    # deploy into system env
    deploy_dummy_stack(client, charm_series)

    # deploy into hosted envs
    with hosted_environment(client, log_dir, 'env1') as env1_client:
        deploy_dummy_stack(env1_client, charm_series)
        with hosted_environment(client, log_dir, 'env2') as env2_client:
            deploy_dummy_stack(env2_client, charm_series)
            # check all the services can talk
            check_services(client)
            check_services(env1_client)
            check_services(env2_client)


def assess_destroy_current(client):
    model_name = client.model_name
    new_model = client.add_model('delete-me')
    new_model.switch('delete-me')
    new_model.destroy_model()
    new_model.show_controller()
    client.switch(model_name)


@contextmanager
def multimodel_setup(args):
    """
    Sets up the juju client and its environment.

    Returns return client, charm_prefix and base_env.
    """
    base_env = args.env
    configure_logging(args.verbose)
    # TODO(gz): Logic from deploy_stack, and precise is a bad default series?
    series = args.series
    if series is None:
        series = 'precise'
    charm_series = series
    client = client_from_config(base_env, args.juju_bin, args.debug,
                                soft_deadline=args.deadline)
    with boot_context(
            args.temp_env_name,
            client,
            args.bootstrap_host,
            args.machine,
            args.series,
            args.arch,
            args.agent_url,
            args.agent_stream,
            args.logs, args.keep_env,
            upload_tools=False,
            region=args.region,
            ):
        yield client, charm_series, base_env


def env_token(env_name):
    return env_name + get_random_string()


@contextmanager
def hosted_environment(system_client, log_dir, suffix):
    env_name = '{}-{}'.format(system_client.env.environment, suffix)
    client = system_client.add_model(env_name)
    try:
        yield client
    except:
        logging.exception(
            'Exception while environment "{}" active'.format(
                client.env.environment))
        sys.exit(1)
    finally:
        safe_print_status(client)
        hosted_log_dir = os.path.join(log_dir, suffix)
        ensure_dir(hosted_log_dir)
        dump_env_logs(client, None, hosted_log_dir)
        client.destroy_model()


def check_services(client):
    token = env_token(client.env.environment)
    client.set_config('dummy-source', {'token': token})
    print_now("checking services in " + client.env.environment)
    check_token(client, token)


def parse_args(argv=None):
    """Parse all arguments."""
    parser = ArgumentParser()
    add_basic_testing_arguments(
        parser, using_jes=True, deadline=True, existing=False)
    return parser.parse_args(argv)


def main():
    args = parse_args()
    with multimodel_setup(args) as (client, charm_series, base_env):
        assess_multimodel_deploy(client, charm_series, args.logs, base_env)
        assess_destroy_current(client)


if __name__ == '__main__':
    main()
