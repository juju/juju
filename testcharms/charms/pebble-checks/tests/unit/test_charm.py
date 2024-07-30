import unittest
import unittest.mock

import ops
import ops.pebble
import ops.testing
from charm import PebbleChecksCharm


class TestCharm(unittest.TestCase):
    def setUp(self):
        self.harness = ops.testing.Harness(PebbleChecksCharm)
        self.addCleanup(self.harness.cleanup)
        self.harness.set_can_connect("ubuntu", True)
        self.harness.begin()

    def test_check_failed(self):
        def get_change(_: ops.pebble.Client, change_id: str):
            return ops.pebble.Change.from_dict(
                {
                    "id": change_id,
                    "kind": ops.pebble.ChangeKind.PERFORM_CHECK.value,
                    "summary": "",
                    "status": ops.pebble.ChangeStatus.ERROR.value,
                    "ready": False,
                    "spawn-time": "2021-02-10T04:36:22.118970777Z",
                }
            )

        with unittest.mock.patch.object(
            ops.testing._TestingPebbleClient, "get_change", get_change
        ):
            self.harness.pebble_notify(
                "ubuntu",
                "123",
                type=ops.pebble.NoticeType.CHANGE_UPDATE,
                data={"kind": "perform-check", "check-name": "exec-check"},
            )
        container = self.harness.charm.model.unit.get_container("ubuntu")
        self.harness.charm.on["ubuntu"].pebble_check_failed.emit(container, "exec-check")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.MaintenanceStatus("check failed: exec-check"),
        )

    def test_check_recovered(self):
        def get_change(_: ops.pebble.Client, change_id: str):
            return ops.pebble.Change.from_dict(
                {
                    "id": change_id,
                    "kind": ops.pebble.ChangeKind.RECOVER_CHECK.value,
                    "summary": "",
                    "status": ops.pebble.ChangeStatus.DONE.value,
                    "ready": False,
                    "spawn-time": "2021-02-10T04:36:23.118970777Z",
                }
            )

        with unittest.mock.patch.object(
            ops.testing._TestingPebbleClient, "get_change", get_change
        ):
            self.harness.pebble_notify(
                "ubuntu",
                "123",
                type=ops.pebble.NoticeType.CHANGE_UPDATE,
                data={"kind": "recover-check", "check-name": "exec-check"},
            )
        container = self.harness.charm.model.unit.get_container("ubuntu")
        self.harness.charm.on["ubuntu"].pebble_check_recovered.emit(container, "exec-check")
        self.assertEqual(
            self.harness.model.unit.status,
            ops.ActiveStatus("check recovered: exec-check"),
        )
