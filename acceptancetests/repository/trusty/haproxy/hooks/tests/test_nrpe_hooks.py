from testtools import TestCase
from mock import call, patch, MagicMock

import hooks


class NRPEHooksTest(TestCase):

    @patch('hooks.install_nrpe_scripts')
    @patch('charmhelpers.contrib.charmsupport.nrpe.NRPE')
    def test_update_nrpe_config(self, nrpe, install_nrpe_scripts):
        nrpe_compat = MagicMock()
        nrpe_compat.checks = [MagicMock(shortname="haproxy"),
                              MagicMock(shortname="haproxy_queue")]
        nrpe.return_value = nrpe_compat

        hooks.update_nrpe_config()

        self.assertEqual(
            nrpe_compat.mock_calls,
            [call.add_check('haproxy', 'Check HAProxy', 'check_haproxy.sh'),
             call.add_check('haproxy_queue', 'Check HAProxy queue depth',
                            'check_haproxy_queue_depth.sh'),
             call.write()])
