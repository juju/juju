#!/usr/bin/env python3

import argparse
import asyncio
import logging
import os
import sys

from juju.client.connection import Connection
from juju.client import watcher

from deploy_stack import (
    BootstrapManager,
    get_random_string,
    until_timeout,
    )
from jujucharm import local_charm_path
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )


__metaclass__ = type


log = logging.getLogger("assess_model_change_watcher")
TOKEN = "1234asdf"


def is_config_change_in_event(event):
    message = _get_message(event)
    return all([
        "application" in event,
        "change" in event,
        is_in_dict("config", {"token": TOKEN}, message),
    ])


def _get_message(event):
    for message in event:
        if isinstance(message, dict):
            return message
    return None


def is_in_dict(key, value, items):
    return items.get(key) == value


async def listen_to_watcher(event_found, conn, future):
    logging.info("Starting to listen for the watcher.")
    allwatcher = watcher.AllWatcher()
    allwatcher.connect(conn)
    for _ in until_timeout(120):
        logging.info("Listening for events...")
        change = await allwatcher.Next()
        for delta in change.deltas:
            logging.info("Event received: {}".format(str(delta.deltas)))
            if event_found(delta.deltas) is True:
                await allwatcher.Stop()
                await conn.close()
                logging.info("Event found: {}".format(str(delta.deltas)))
                future.set_result(True)
                return
    await allwatcher.Stop()
    await conn.close()
    logging.warning("Event not found.")
    future.set_result(False)


def run_listener(client, event, juju_bin):
    logging.info("Running the watcher")
    loop = asyncio.get_event_loop()
    future = asyncio.Future()

    logging.info("Connect to the current model.")
    os.environ['JUJU_DATA'] = client.env.juju_home
    os.environ['PATH'] = "{}{}{}".format(
        juju_bin, os.pathsep, os.environ.get('PATH', ''))
    conn = loop.run_until_complete(Connection.connect_current())
    logging.info("Connected to the current model.")

    asyncio.ensure_future(listen_to_watcher(event, conn, future))
    return loop, future, conn


def assess_model_change_watcher(client, charm_series, juju_bin):
    logging.info("Deploying a charm.")
    # charm = local_charm_path(
    #     charm='dummy-source', juju_ver=client.version, series=charm_series,
    #     platform='ubuntu')
    # client.deploy(charm)
    # client.wait_for_started()

    loop, future, conn = run_listener(client, is_config_change_in_event, juju_bin)

    #logging.info("Making config change.")
    #client.set_config('dummy-source', {'token': TOKEN})

    loop.run_until_complete(future)
    result = future.result()
    if result is not True:
        raise JujuAssertionError("Config change was not sent.")
    loop.close()


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Model change watcher")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_model_change_watcher(bs_manager.client, bs_manager.series,
                                    args.juju_bin)
    return 0



def testing_ONLY():
    class Object(object):
        pass
    c = Object()
    c.env = Object()
    args=parse_args(None)
    configure_logging(args.verbose)
    c.env.juju_home = '/home/me/cloud-city'
    assess_model_change_watcher(c, None, '/usr/bin/juju')

if __name__ == '__main__':
    testing_ONLY()
    #sys.exit(main())
