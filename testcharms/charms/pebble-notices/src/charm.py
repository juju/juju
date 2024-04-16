#!/usr/bin/env python3
"""Charm to test Pebble Notices."""

import logging

import ops

logger = logging.getLogger(__name__)


class PebbleNoticesCharm(ops.CharmBase):
    """Charm to test Pebble Notices."""

    def __init__(self, *args):
        super().__init__(*args)
        self.framework.observe(self.on["redis"].pebble_ready, self._on_pebble_ready)
        self.framework.observe(self.on["redis"].pebble_custom_notice, self._on_custom_notice)

    def _on_pebble_ready(self, event):
        self.unit.status = ops.ActiveStatus()

    def _on_custom_notice(self, event):
        notice_id = event.notice.id
        notice_type = event.notice.type.value  # .value: "custom", not "NoticeType.CUSTOM"
        notice_key = event.notice.key
        logger.info(f"_on_custom_notice: id={notice_id} type={notice_type} key={notice_key}")

        # Don't include the (arbitrary) ID in the status message
        self.unit.status = ops.MaintenanceStatus(f"notice type={notice_type} key={notice_key}")


if __name__ == "__main__":
    ops.main(PebbleNoticesCharm)
