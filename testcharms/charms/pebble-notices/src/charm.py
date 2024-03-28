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

        # TODO(benhoyt): update to use pebble_change_updated once ops supports that:
        #                https://github.com/canonical/operator/pull/1170
        import os
        import pathlib

        dispatch_path = pathlib.Path(os.environ.get("JUJU_DISPATCH_PATH", ""))
        event_name = dispatch_path.name.replace("-", "_")
        logger.info(f"__init__: path={dispatch_path} event={event_name}")
        if event_name == "redis_pebble_change_updated":
            event = ops.PebbleNoticeEvent(
                None,
                self.unit.get_container(os.environ["JUJU_WORKLOAD_NAME"]),
                os.environ["JUJU_NOTICE_ID"],
                os.environ["JUJU_NOTICE_TYPE"],
                os.environ["JUJU_NOTICE_KEY"],
            )
            self._on_change_updated(event)

    def _on_pebble_ready(self, event):
        self.unit.status = ops.ActiveStatus()

    def _on_custom_notice(self, event):
        notice_id = event.notice.id
        notice_type = event.notice.type.value  # .value: "custom", not "NoticeType.CUSTOM"
        notice_key = event.notice.key
        logger.info(f"_on_custom_notice: id={notice_id} type={notice_type} key={notice_key}")

        # Don't include the (arbitrary) ID in the status message
        self.unit.status = ops.MaintenanceStatus(f"notice type={notice_type} key={notice_key}")

    def _on_change_updated(self, event):
        notice_id = event.notice.id
        notice_type = (
            event.notice.type if isinstance(event.notice.type, str) else event.notice.type.value
        )
        notice_key = event.notice.key
        logger.info(f"_on_change_updated: id={notice_id} type={notice_type} key={notice_key}")

        change = event.workload.pebble.get_change(notice_key)
        self.unit.status = ops.MaintenanceStatus(
            f"notice type={notice_type} kind={change.kind} status={change.status}"
        )


if __name__ == "__main__":
    ops.main(PebbleNoticesCharm)
