
from testtools import TestCase
from mock import patch
from charmhelpers.core.hookenv import Config

import hooks


class StatisticsRelationTest(TestCase):

    def setUp(self):
        super(StatisticsRelationTest, self).setUp()
        config = Config(**{"monitoring_port": 10001,
                           "monitoring_username": "mon_user",
                           "enable_monitoring": True})
        self.config_get = self.patch_hook("config_get")
        self.config_get.return_value = config
        # patch changed and save methods to do nothing
        self.config_get().changed = lambda x: False
        self.config_get().save = lambda: None
        self.get_monitoring_password = \
            self.patch_hook("get_monitoring_password")
        self.get_monitoring_password.return_value = "this-is-a-secret"
        self.relation_set = self.patch_hook("relation_set")
        self.get_relation_ids = self.patch_hook("get_relation_ids")
        self.get_relation_ids.return_value = ['__stats-rel-id__']
        self.log = self.patch_hook("log")

    def patch_hook(self, hook_name):
        mock_controller = patch.object(hooks, hook_name)
        mock = mock_controller.start()
        self.addCleanup(mock_controller.stop)
        return mock

    def test_relation_joined(self):
        hooks.statistics_interface()
        self.get_relation_ids.assert_called_once_with('statistics')
        self.relation_set.assert_called_once_with(
            relation_id=self.get_relation_ids()[0],
            enabled=True,
            port=10001,
            password="this-is-a-secret",
            user="mon_user")

    def test_relation_joined_monitoring_disabled(self):
        self.config_get.return_value['enable_monitoring'] = False
        hooks.statistics_interface()
        self.get_relation_ids.assert_called_once_with('statistics')
        self.relation_set.assert_called_once_with(
            relation_id=self.get_relation_ids()[0],
            enabled=False)

    def test_called_on_config_change(self):
        config_changed = self.patch_hook('config_changed')
        update_nrpe_config = self.patch_hook('update_nrpe_config')
        statistics_interface = self.patch_hook('statistics_interface')
        hooks.main('config-changed')
        config_changed.assert_called_once_with()
        update_nrpe_config.assert_called_once_with()
        statistics_interface.assert_called_once_with()
