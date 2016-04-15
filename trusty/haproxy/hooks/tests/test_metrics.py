import textwrap

from testtools import TestCase
from mock import patch, mock_open

import hooks


class MetricsTestCase(TestCase):

    def add_patch(self, *args, **kwargs):
        p = patch(*args, **kwargs)
        self.addCleanup(p.stop)
        return p.start()

    def setUp(self):
        super(MetricsTestCase, self).setUp()

        self.open = mock_open()
        self.add_patch("hooks.open", self.open, create=True)
        self.copy2 = self.add_patch("shutil.copy2")
        self.config_get = self.add_patch("hooks.config_get")
        self.local_unit = self.add_patch("hooks.local_unit")
        self.log = self.add_patch("hooks.log")
        self.apt_install = self.add_patch("hooks.apt_install")
        self.get_monitoring_password = self.add_patch(
            "hooks.get_monitoring_password")

        self.config_get.return_value = {
            'metrics_sample_interval': 5,
            'metrics_prefix': 'prefix.$UNIT',
            'metrics_target': 'localhost:4321',
            'enable_monitoring': True,
            'monitoring_port': '1234',
            'monitoring_username': 'monitor',
            'monitoring_password': 'monitorpass',
        }
        self.local_unit.return_value = "unit/0"

    @patch('hooks.os.unlink')
    def test_write_metrics_cronjob_disabled_no_monitoring(self, mock_unlink):
        self.config_get.return_value['enable_monitoring'] = False
        hooks.write_metrics_cronjob('/script', '/cron')
        self.assertEqual(mock_unlink.mock_calls[0][1][0], '/cron')

    @patch('hooks.os.unlink')
    def test_write_metrics_cronjob_disabled_no_target(self, mock_unlink):
        self.config_get.return_value['metrics_target'] = ''
        hooks.write_metrics_cronjob('/script', '/cron')
        self.assertEqual(mock_unlink.mock_calls[0][1][0], '/cron')

    @patch('hooks.os.unlink')
    def test_write_metrics_cronjob_disabled_bad_target(self, mock_unlink):
        self.config_get.return_value['metrics_target'] = 'sadfsadf'
        hooks.write_metrics_cronjob('/script', '/cron')
        self.assertEqual(mock_unlink.mock_calls[0][1][0], '/cron')

    def test_write_metrics_cronjob_enabled(self):
        self.get_monitoring_password.return_value = 'monitorpass'
        self.config_get.return_value['metrics_target'] = 'localhost:4321'

        hooks.write_metrics_cronjob('/script', '/cron')

        cron_open_args = self.open.mock_calls[0][1]
        self.assertEqual(cron_open_args, ('/cron', 'w'))

        cron_write = self.open.mock_calls[2][1][0]
        expected_cron = textwrap.dedent("""
           # crontab for pushing haproxy metrics to statsd
           */5 * * * * root bash /script prefix.unit-0 5min localhost:1234\
 monitor:monitorpass | python -c "import socket, sys; sock =\
 socket.socket(socket.AF_INET, socket.SOCK_DGRAM); map(lambda line:\
 sock.sendto(line, ('localhost', 4321)), sys.stdin)"
        """).lstrip()
        self.assertEqual(expected_cron, cron_write)
