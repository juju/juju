import os
import unittest

import ops
import ops.testing
from charm import PebbleNoticesCharm


class TestCharm(unittest.TestCase):
    def setUp(self):
        self.harness = ops.testing.Harness(PebbleNoticesCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.begin()
        self._next_notice_id = 1

    def test_pebble_ready(self):
        self.harness.container_pebble_ready("redis")
        self.assertEqual(self.harness.model.unit.status, ops.ActiveStatus())

    def test_custom_notice(self):
        os.environ["JUJU_DISPATCH_PATH"] = "hooks/redis-pebble-custom-notice"
        self.addCleanup(os.environ.__delitem__, "JUJU_DISPATCH_PATH")
        self.addCleanup(os.environ.__delitem__, "JUJU_NOTICE_ID")
        self.addCleanup(os.environ.__delitem__, "JUJU_NOTICE_TYPE")
        self.addCleanup(os.environ.__delitem__, "JUJU_NOTICE_KEY")

        self._pebble_notify("redis", "example.com/foo")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("notice type=custom key=example.com/foo"),
        )

        self._pebble_notify("redis", "ubuntu.com/bar/buzz")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("notice type=custom key=ubuntu.com/bar/buzz"),
        )

    def _pebble_notify(self, container_name, notice_key):
        os.environ["JUJU_NOTICE_ID"] = str(self._next_notice_id)
        self._next_notice_id += 1
        os.environ["JUJU_NOTICE_TYPE"] = "custom"
        os.environ["JUJU_NOTICE_KEY"] = notice_key
        self.harness.charm._on_custom_notice(None)

        # TODO(benhoyt): update to use pebble_custom_notice once ops supports that
        # container = self.harness.model.unit.get_container(container_name)
        # self.harness.charm.on[container_name].pebble_custom_notice.emit(
        #     container, "custom", notice_key
        # )
