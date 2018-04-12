from jujupy.fake import FakeEnvironmentState
from tests import TestCase


class TestEnvironmentState(TestCase):

    def test_get_status_dict_series(self):
        state = FakeEnvironmentState()
        state.add_container('lxd')
        status_dict = state.get_status_dict()
        machine_0 = status_dict['machines']['0']
        self.assertEqual(machine_0['series'], 'angsty')
        lxd_0 = machine_0['containers']['0/lxd/0']
        self.assertEqual(lxd_0['series'], 'angsty')
