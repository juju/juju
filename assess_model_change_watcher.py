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
    until_timeout,
    )
from jujucharm import local_charm_path
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )


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
    all_watcher = watcher.AllWatcher()
    all_watcher.connect(conn)
    for _ in until_timeout(120):
        logging.info("Listening for events...")
        change = await all_watcher.Next()
        for delta in change.deltas:
            logging.info("Event received: {}".format(str(delta.deltas)))
            if event_found(delta.deltas) is True:
                await all_watcher.Stop()
                await conn.close()
                logging.info("Event found: {}".format(str(delta.deltas)))
                future.set_result(True)
                return

    await all_watcher.Stop()
    await conn.close()
    logging.warning("Event not found.")
    future.set_result(False)


def run_listener(client, event, juju_bin):
    logging.info("Running listener.")
    loop = asyncio.get_event_loop()
    future = asyncio.Future()

    logging.info("Connect to the current model.")
    os.environ['JUJU_DATA'] = client.env.juju_home
    os.environ['PATH'] = "{}{}{}".format(
        juju_bin, os.pathsep, os.environ.get('PATH', ''))
    conn = loop.run_until_complete(Connection.connect_current())
    logging.info("Connected to the current model.")

    asyncio.ensure_future(listen_to_watcher(event, conn, future))
    return loop, future


def assess_model_change_watcher(client, charm_series, juju_bin):
    charm = local_charm_path(
        charm='dummy-source', juju_ver=client.version, series=charm_series,
        platform='ubuntu')
    client.deploy(charm)
    client.wait_for_started()

    loop, future = run_listener(client, is_config_change_in_event, juju_bin)

    logging.info("Making config change.")
    client.set_config('dummy-source', {'token': TOKEN})

    loop.run_until_complete(future)
    result = future.result()
    if result is not True:
        raise JujuAssertionError("Config change event was not sent.")
    loop.close()


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Assess config change.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_model_change_watcher(
            bs_manager.client, bs_manager.series, args.juju_bin)
    return 0


if __name__ == '__main__':
    sys.exit(main())
