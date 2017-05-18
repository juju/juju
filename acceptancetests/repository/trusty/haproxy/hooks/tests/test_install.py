from mock import patch
import os
from testtools import TestCase

import hooks


class InstallTests(TestCase):

    def setUp(self):
        super(InstallTests, self).setUp()
        self.add_source = self.patch_hook('add_source')
        self.apt_update = self.patch_hook('apt_update')
        self.apt_install = self.patch_hook('apt_install')
        self.ensure_package_status = self.patch_hook('ensure_package_status')
        self.enable_haproxy = self.patch_hook('enable_haproxy')
        self.config_get = self.patch_hook('config_get')
        path_exists = patch.object(os.path, "exists")
        self.path_exists = path_exists.start()
        self.path_exists.return_value = True
        self.addCleanup(path_exists.stop)

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    @patch('os.mkdir')
    def test_makes_config_dir(self, mkdir):
        self.path_exists.return_value = False
        hooks.install_hook()
        self.path_exists.assert_called_once_with(
            hooks.default_haproxy_service_config_dir)
        mkdir.assert_called_once_with(
            hooks.default_haproxy_service_config_dir, 0600)

    @patch('os.mkdir')
    def test_config_dir_already_exists(self, mkdir):
        hooks.install_hook()
        self.path_exists.assert_called_once_with(
            hooks.default_haproxy_service_config_dir)
        self.assertFalse(mkdir.called)

    def test_install_packages(self):
        hooks.install_hook()
        calls = self.apt_install.call_args_list
        self.assertEqual((['haproxy', 'python-jinja2'],), calls[0][0])
        self.assertEqual({'fatal': True}, calls[0][1])
        self.assertEqual(
            (['python-pyasn1', 'python-pyasn1-modules'],), calls[1][0])
        self.assertEqual({'fatal': False}, calls[1][1])

    def test_add_source(self):
        hooks.install_hook()
        self.config_get.assert_called_once_with()
        self.add_source.assert_called_once_with(
            self.config_get.return_value.get("source"),
            self.config_get.return_value.get("key"))

    def test_apt_update(self):
        hooks.install_hook()
        self.apt_update.assert_called_once_with(fatal=True)

    def test_add_source_with_backports(self):
        self.config_get.return_value = {
            'source': 'backports', 'package_status': 'install'}
        with patch("hooks.lsb_release") as lsb_release:
            lsb_release.return_value = {'DISTRIB_CODENAME': 'trusty'}
            with patch("hooks.add_backports_preferences") as add_apt_prefs:
                hooks.install_hook()
                add_apt_prefs.assert_called_once_with('trusty')
        self.config_get.assert_called_once_with()
        source = ("deb http://archive.ubuntu.com/ubuntu trusty-backports "
                  "main restricted universe multiverse")
        self.add_source.assert_called_once_with(
            source,
            self.config_get.return_value.get("key"))

    def test_ensures_package_status(self):
        hooks.install_hook()
        self.config_get.assert_called_once_with()
        self.ensure_package_status.assert_called_once_with(
            hooks.service_affecting_packages,
            self.config_get.return_value["package_status"])

    def test_calls_enable_haproxy(self):
        hooks.install_hook()
        self.enable_haproxy.assert_called_once_with()
