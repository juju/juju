import unittest
import unittest.mock

import ops
import ops.testing
from charm import PebbleNoticesCharm
from ops import pebble


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

    @unittest.mock.patch("ops.testing._TestingPebbleClient.get_change")
    def test_change_updated(self, mock_get_change):
        # TODO(benhoyt): update to use pebble_change_updated once ops supports that:
        #                https://github.com/canonical/operator/pull/1170

        import os

        os.environ["JUJU_DISPATCH_PATH"] = "hooks/redis-pebble-change-updated"
        self.addCleanup(os.environ.__delitem__, "JUJU_DISPATCH_PATH")
        self.addCleanup(os.environ.__delitem__, "JUJU_NOTICE_ID")
        self.addCleanup(os.environ.__delitem__, "JUJU_NOTICE_TYPE")
        self.addCleanup(os.environ.__delitem__, "JUJU_NOTICE_KEY")

        mock_get_change.return_value = pebble.Change.from_dict(
            {
                "id": "1",
                "kind": "exec",
                "summary": "",
                "status": "Doing",
                "ready": False,
                "spawn-time": "2021-01-28T14:37:02.247202105+13:00",
            }
        )
        self._pebble_notify_change_updated("redis", "123")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("notice type=change-update kind=exec status=Doing"),
        )
        mock_get_change.assert_called_once_with("123")

        mock_get_change.reset_mock()
        mock_get_change.return_value = pebble.Change.from_dict(
            {
                "id": "2",
                "kind": "changeroo",
                "summary": "",
                "status": "Done",
                "ready": True,
                "spawn-time": "2024-01-28T14:37:02.247202105+13:00",
                "ready-time": "2024-01-28T14:37:04.291517768+13:00",
            }
        )
        self._pebble_notify_change_updated("redis", "42")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("notice type=change-update kind=changeroo status=Done"),
        )
        mock_get_change.assert_called_once_with("42")

    def _pebble_notify_change_updated(self, container_name, notice_key):
        import os

        os.environ["JUJU_NOTICE_ID"] = notice_id = str(self._next_notice_id)
        self._next_notice_id += 1
        os.environ["JUJU_NOTICE_TYPE"] = notice_type = "change-update"
        os.environ["JUJU_NOTICE_KEY"] = notice_key
        event = ops.PebbleNoticeEvent(
            None,
            self.harness.model.unit.get_container(container_name),
            notice_id,
            notice_type,
            notice_key,
        )
        self.harness.charm._on_change_updated(event)
