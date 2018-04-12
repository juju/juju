from mock import patch
from testtools import TestCase

import hooks


class UpgradeCharmTests(TestCase):

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    def test_calls_hooks(self):
        install_hook = self.patch_hook('install_hook')
        config_changed = self.patch_hook('config_changed')
        update_nrpe_config = self.patch_hook('update_nrpe_config')
        hooks.main('upgrade-charm')
        install_hook.assert_called_once_with()
        config_changed.assert_called_once_with()
        update_nrpe_config.assert_called_once_with()
