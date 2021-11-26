#!/usr/bin/python3

import asyncio
import logging

from juju.client import client
from juju.model import Controller


async def watch():
    controller = Controller()
    await controller.connect()

    api = client.ControllerFacade.from_connection(controller.connection())
    watcher = client.AllModelWatcherFacade.from_connection(
        controller.connection())
    result = await api.WatchAllModels()
    watcher.Id = result.watcher_id

    while True:
        change = await watcher.Next()
        for delta in change.deltas:
            print("-- change --\n{}\n".format(delta))


if __name__ == '__main__':
    logging.basicConfig(level=logging.INFO)
    ws_logger = logging.getLogger('websockets.protocol')
    ws_logger.setLevel(logging.INFO)
    asyncio.run(watch())
