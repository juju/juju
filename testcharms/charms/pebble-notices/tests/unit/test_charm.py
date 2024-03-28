import unittest

import ops
import ops.testing
from charm import PebbleNoticesCharm


class TestCharm(unittest.TestCase):
    def setUp(self):
        self.harness = ops.testing.Harness(PebbleNoticesCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.begin()

    def test_pebble_ready(self):
        self.harness.container_pebble_ready("redis")
        self.assertEqual(self.harness.model.unit.status, ops.ActiveStatus())

    def test_custom_notice(self):
        self.harness.pebble_notify("redis", "example.com/foo")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("notice type=custom key=example.com/foo"),
        )

        self.harness.pebble_notify("redis", "ubuntu.com/bar/buzz")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("notice type=custom key=ubuntu.com/bar/buzz"),
        )
