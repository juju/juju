#!/usr/bin/env python3
"""Charm to test Pebble Notices."""

import logging
import os
import pathlib

import ops

logger = logging.getLogger(__name__)


class PebbleNoticesCharm(ops.CharmBase):
    """Charm to test Pebble Notices."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on["redis"].pebble_ready, self._on_pebble_ready)

        # TODO(benhoyt): update to use pebble_custom_notice once ops supports that
        # self.framework.observe(self.on["redis"].pebble_custom_notice, self._on_custom_notice)

        dispatch_path = pathlib.Path(os.environ.get("JUJU_DISPATCH_PATH", ""))
        event_name = dispatch_path.name.replace("-", "_")
        logger.info(f"__init__: path={dispatch_path} event={event_name}")
        if event_name == "redis_pebble_custom_notice":
            self._on_custom_notice(None)

    def _on_pebble_ready(self, event):
        self.unit.status = ops.ActiveStatus()

    def _on_custom_notice(self, event):
        notice_id = os.environ["JUJU_NOTICE_ID"]
        notice_type = os.environ["JUJU_NOTICE_TYPE"]
        notice_key = os.environ["JUJU_NOTICE_KEY"]
        logger.info(f"_on_custom_notice: id={notice_id} type={notice_type} key={notice_key}")

        # Don't include the (arbitrary) ID in the status message
        self.unit.status = ops.MaintenanceStatus(f"notice type={notice_type} key={notice_key}")


if __name__ == "__main__":
    ops.main(PebbleNoticesCharm)
