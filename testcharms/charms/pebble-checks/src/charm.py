#!/usr/bin/env python3
"""Charm to test Pebble Notices."""

import logging

import ops

logger = logging.getLogger(__name__)


class PebbleChecksCharm(ops.CharmBase):
    """Charm to test Pebble check events."""

    def __init__(self, framework):
        super().__init__(framework)
        framework.observe(self.on["ubuntu"].pebble_ready, self._on_ready)
        framework.observe(self.on["ubuntu"].pebble_check_failed, self._on_check_failed)
        framework.observe(self.on["ubuntu"].pebble_check_recovered, self._on_check_recovered)

    def _on_ready(self, _):
        layer = ops.pebble.Layer(
            {
                "summary": "Dummy layer",
                "description": "A layer with a check that can fail and recover",
                "services": {
                    "sleep": {
                        "override": "replace",
                        "summary": "zzzzz",
                        "command": "sleep 600",
                        "startup": "enabled",
                    }
                },
                "checks": {
                    "exec-check": {
                        "override": "replace",
                        "period": "0.1s",
                        "threshold": 1,
                        "exec": {
                            "command": "/usr/bin/ls /trigger/",
                        },
                    },
                },
            }
        )
        self.unit.containers["ubuntu"].add_layer("sleepy", layer, combine=True)
        self.unit.containers["ubuntu"].replan()

    def _on_check_failed(self, event):
        logger.info("_on_check_failed: name=%s", event.info.name)
        self.unit.status = ops.MaintenanceStatus(f"check failed: {event.info.name}")

    def _on_check_recovered(self, event):
        logger.info("_on_check_recovered: name=%s", event.info.name)
        self.unit.status = ops.ActiveStatus(f"check recovered: {event.info.name}")


if __name__ == "__main__":
    ops.main(PebbleChecksCharm)
