#!/usr/bin/env python3
# Copyright 2021 Ben
# See LICENSE file for licensing details.

"""Charm the service."""

import logging

from ops.charm import CharmBase
from ops.main import main
from ops.model import ActiveStatus, ModelError
from ops.pebble import ServiceStatus

logger = logging.getLogger(__name__)


class SnappassTestCharm(CharmBase):
    """Charm the service."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on.snappass_pebble_ready, self._on_snappass_pebble_ready)
        self.framework.observe(self.on.redis_pebble_ready, self._on_redis_pebble_ready)

    def _on_snappass_pebble_ready(self, event):
        logger.info("_on_snappass_pebble_ready")
        container = self.unit.containers["redis"]

        test_file = self.model.resources.fetch("test-file")
        target_dir = test_file.parent
        container.make_dir(str(target_dir), make_parents=True)
        container.push(str(test_file), test_file.read_bytes())
        logger.info("files at %s are %s", str(target_dir), container.list_files(str(test_file)))

        if self._is_running(container, "redis"):
            logger.info("redis already started")
            self._start_snappass()

    def _start_snappass(self):
        logger.info("_start_snappass")
        container = self.unit.containers["snappass"]

        if self._is_running(container, "snappass"):
            logger.info("snappass already started")
            return

        snappass_layer = {
            "summary": "snappass layer",
            "description": "snappass layer",
            "services": {
                "snappass": {
                    "override": "replace",
                    "summary": "snappass service",
                    "command": "snappass",
                    "startup": "enabled",
                }
            },
        }

        container.add_layer("snappass", snappass_layer, combine=True)
        container.autostart()
        self.unit.status = ActiveStatus("snappass started")

    def _on_redis_pebble_ready(self, event):
        logger.info("_on_redis_pebble_ready")
        container = event.workload

        if self._is_running(container, "redis"):
            logger.info("redis already started")
            return

        redis_layer = {
            "summary": "redis layer",
            "description": "redis layer",
            "services": {
                "redis": {
                    "override": "replace",
                    "summary": "redis service",
                    "command": "redis-server",
                    "startup": "enabled",
                }
            },
        }

        container.add_layer("redis", redis_layer, combine=True)
        container.autostart()
        self.unit.status = ActiveStatus("redis started")
        # Prod snappass to start
        self._start_snappass()

    def _is_running(self, container, service):
        """Determine if a given service is running in a given container."""
        try:
            service = container.get_service(service)
        except ModelError:
            return False
        return service.current == ServiceStatus.ACTIVE


if __name__ == "__main__":
    main(SnappassTestCharm)
