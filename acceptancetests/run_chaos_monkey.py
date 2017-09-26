#!/usr/bin/env python
from argparse import ArgumentParser
from datetime import datetime
import logging
import sys
from time import sleep

from chaos import MonkeyRunner
from jujupy import (
    client_from_config,
)
from utility import configure_logging


__metaclass__ = type


def run_while_healthy_or_timeout(monkey):
    logging.debug('run_while_healthy_or_timeout')
    while monkey.is_healthy():
        logging.debug('Unleashing chaos.')
        monkey.unleash_once()
        monkey.wait_for_chaos()
        if datetime.now() > monkey.expire_time:
            logging.debug(
                'Reached run timeout, all done running chaos.')
            break
        if monkey.pause_timeout:
            logging.debug(
                'Pausing {} seconds after running chaos.'.format(
                    monkey.pause_timeout))
            sleep(monkey.pause_timeout)
    else:
        logging.error('The health check reported an error: {}'.format(
            monkey.health_checker))
        sys.exit(1)


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('env', help='The name of the environment.')
    parser.add_argument('service', help='A service name to monkey with.')
    parser.add_argument(
        'health_checker',
        help='A binary for checking the health of the environment.')
    parser.add_argument(
        '-et', '--enablement-timeout', default=30, type=int,
        help="Enablement timeout in seconds.", metavar='SECONDS')
    parser.add_argument(
        '-tt', '--total-timeout', type=int, help="Total timeout in seconds.",
        metavar='SECONDS')
    parser.add_argument(
        '-pt', '--pause-timeout', default=0, type=int,
        help="Pause timeout in seconds.", metavar='SECONDS')
    args = parser.parse_args(argv)
    if not args.total_timeout:
        args.total_timeout = args.enablement_timeout
    if args.enablement_timeout > args.total_timeout:
        parser.error("total-timeout can not be less than "
                     "enablement-timeout.")
    if args.total_timeout <= 0:
        parser.error("Invalid total-timeout value: timeout must be "
                     "greater than zero.")
    if args.enablement_timeout < 0:
        parser.error("Invalid enablement-timeout value: timeout must be "
                     "zero or greater.")
    return args


def main():
    """ Deploy and run chaos monkey, while checking env health.

    The Chaos Monkey is deployed into the environment and related to
    the specified service. Juju actions are then used to run one chaos
    operation at a time. After each operation, the provided health
    check script is executed, to ensure the Juju environment or
    software stack is still healthy.
    """
    configure_logging(logging.INFO)
    args = get_args()
    client = client_from_config(args.env, None)
    monkey_runner = MonkeyRunner(
        args.env, client, service=args.service,
        health_checker=args.health_checker,
        enablement_timeout=args.enablement_timeout,
        pause_timeout=args.pause_timeout,
        total_timeout=args.total_timeout)
    logging.info("Chaos Monkey Start.")
    monkey_runner.deploy_chaos_monkey()
    run_while_healthy_or_timeout(monkey_runner)
    logging.info("Chaos Monkey Complete.")


if __name__ == '__main__':
    main()
