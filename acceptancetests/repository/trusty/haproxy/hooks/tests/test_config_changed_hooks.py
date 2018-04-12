import sys
import base64
import os

from testtools import TestCase
from mock import patch, call

import hooks
from utils_for_tests import patch_open


class ConfigChangedTest(TestCase):

    def setUp(self):
        super(ConfigChangedTest, self).setUp()
        self.config_get = self.patch_hook("config_get")
        self.config_get().changed.return_value = False
        self.get_service_ports = self.patch_hook("get_service_ports")
        self.get_listen_stanzas = self.patch_hook("get_listen_stanzas")
        self.create_haproxy_globals = self.patch_hook(
            "create_haproxy_globals")
        self.create_haproxy_defaults = self.patch_hook(
            "create_haproxy_defaults")
        self.remove_services = self.patch_hook("remove_services")
        self.create_services = self.patch_hook("create_services")
        self.load_services = self.patch_hook("load_services")
        self.construct_haproxy_config = self.patch_hook(
            "construct_haproxy_config")
        self.service_haproxy = self.patch_hook(
            "service_haproxy")
        self.update_sysctl = self.patch_hook(
            "update_sysctl")
        self.update_ssl_cert = self.patch_hook(
            "update_ssl_cert")
        self.notify_website = self.patch_hook("notify_website")
        self.notify_peer = self.patch_hook("notify_peer")
        self.write_metrics_cronjob = self.patch_hook("write_metrics_cronjob")
        self.log = self.patch_hook("log")
        sys_exit = patch.object(sys, "exit")
        self.sys_exit = sys_exit.start()
        self.addCleanup(sys_exit.stop)

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    def test_config_changed_notify_website_changed_stanzas(self):
        self.service_haproxy.return_value = True
        self.get_listen_stanzas.side_effect = (
            (('foo.internal', '1.2.3.4', 123),),
            (('foo.internal', '1.2.3.4', 123),
             ('bar.internal', '1.2.3.5', 234),))

        hooks.config_changed()

        self.notify_website.assert_called_once_with()
        self.notify_peer.assert_called_once_with()

    def test_config_changed_no_notify_website_not_changed(self):
        self.service_haproxy.return_value = True
        self.get_listen_stanzas.side_effect = (
            (('foo.internal', '1.2.3.4', 123),),
            (('foo.internal', '1.2.3.4', 123),))

        hooks.config_changed()

        self.notify_website.assert_not_called()
        self.notify_peer.assert_not_called()

    def test_config_changed_no_notify_website_failed_check(self):
        self.service_haproxy.return_value = False
        self.get_listen_stanzas.side_effect = (
            (('foo.internal', '1.2.3.4', 123),),
            (('foo.internal', '1.2.3.4', 123),
             ('bar.internal', '1.2.3.5', 234),))

        hooks.config_changed()

        self.notify_website.assert_not_called()
        self.notify_peer.assert_not_called()
        self.log.assert_called_once_with(
            "HAProxy configuration check failed, exiting.")
        self.sys_exit.assert_called_once_with(1)

    def test_config_changed_notify_reverseproxy(self):
        """
        If the ssl_cert config value changes, the reverseproxy relations get
        updated.
        """
        config_data = self.config_get()
        config_data.changed.return_value = True
        _notify_reverseproxy = self.patch_hook("_notify_reverseproxy")
        service_restart = self.patch_hook('service_restart')

        hooks.config_changed()
        self.assertIn(call('ssl_cert'), config_data.changed.mock_calls)
        _notify_reverseproxy.assert_called_once_with()
        service_restart.assert_called_once_with('rsyslog')

    def test_config_changed_restart_rsyslog(self):
        """
        If the gloabl_log or source config value changes, rsyslog is
        restarted
        """
        config_data = self.config_get()
        called = []

        def changed(a):
            if a in called or a == 'ssl_cert':
                return False
            called.append(a)
            return True

        config_data.changed.side_effect = changed
        service_restart = self.patch_hook('service_restart')

        hooks.config_changed()
        self.assertIn(call('global_log'), config_data.changed.mock_calls)
        service_restart.assert_called_once_with('rsyslog')
        hooks.config_changed()
        self.assertIn(call('source'), config_data.changed.mock_calls)
        service_restart.assert_called_with('rsyslog')


class HelpersTest(TestCase):
    def test_constructs_haproxy_config(self):
        with patch_open() as (mock_open, mock_file):
            hooks.construct_haproxy_config('foo-globals', 'foo-defaults',
                                           'foo-monitoring', 'foo-services')

            mock_file.write.assert_called_with(
                'foo-globals\n\n'
                'foo-defaults\n\n'
                'foo-monitoring\n\n'
                'foo-services\n\n'
            )
            mock_open.assert_called_with(hooks.default_haproxy_config, 'w')

    def test_constructs_nothing_if_globals_is_none(self):
        with patch_open() as (mock_open, mock_file):
            hooks.construct_haproxy_config(None, 'foo-defaults',
                                           'foo-monitoring', 'foo-services')

            self.assertFalse(mock_open.called)
            self.assertFalse(mock_file.called)

    def test_constructs_nothing_if_defaults_is_none(self):
        with patch_open() as (mock_open, mock_file):
            hooks.construct_haproxy_config('foo-globals', None,
                                           'foo-monitoring', 'foo-services')

            self.assertFalse(mock_open.called)
            self.assertFalse(mock_file.called)

    def test_constructs_haproxy_config_without_optionals(self):
        with patch_open() as (mock_open, mock_file):
            hooks.construct_haproxy_config('foo-globals', 'foo-defaults')

            mock_file.write.assert_called_with(
                'foo-globals\n\n'
                'foo-defaults\n\n'
            )
            mock_open.assert_called_with(hooks.default_haproxy_config, 'w')

    def test_update_ssl_cert_custom_certificate(self):
        config_data = {
            "ssl_cert": base64.b64encode("cert-data\n"),
            "ssl_key": base64.b64encode("key-data\n")}
        with patch("hooks.log"):
            with patch("hooks.write_ssl_pem") as write_ssl_pem_mock:
                hooks.update_ssl_cert(config_data)
                default_pem_path = os.path.join(
                    hooks.default_haproxy_lib_dir, "default.pem")
                write_ssl_pem_mock.assert_called_with(
                    default_pem_path, "cert-data\nkey-data\n")

    def test_update_ssl_cert_selfsigned(self):
        config_data = {"ssl_cert": "SELFSIGNED"}
        with patch("hooks.log"):
            with patch("hooks.get_selfsigned_cert") as selfsigned_mock:
                selfsigned_mock.return_value = "data"
                with patch("hooks.write_ssl_pem") as write_ssl_pem_mock:
                    hooks.update_ssl_cert(config_data)
                    default_pem_path = os.path.join(
                        hooks.default_haproxy_lib_dir, "default.pem")
                    write_ssl_pem_mock.assert_called_with(
                        default_pem_path, "data")
