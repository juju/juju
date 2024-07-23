from collections import defaultdict
from datetime import (
    datetime,
    timedelta,
    )
from dateutil import tz
try:
    from mock import patch
except ImportError:
    from unittest.mock import patch
import types

from jujupy.exceptions import (
    AgentError,
    AgentUnresolvedError,
    AppError,
    ErroredUnit,
    HookFailedError,
    InstallError,
    MachineError,
    ProvisioningError,
    StatusError,
    StuckAllocatingError,
    UnitError,
)
from jujupy.tests.test_client import (
    TestModelClient,
)
from jujupy.status import (
    Status,
    StatusItem
    )
from tests import (
    TestCase,
    FakeHomeTestCase,
)


class TestStatusItem(TestCase):

    @staticmethod
    def make_status_item(status_name, item_name, **kwargs):
        return StatusItem(status_name, item_name, {status_name: kwargs})

    def assertIsType(self, obj, target_type):
        self.assertIs(type(obj), target_type)

    def test_datetime_since(self):
        item = self.make_status_item(StatusItem.JUJU, '0',
                                     since='19 Aug 2016 05:36:42Z')
        target = datetime(2016, 8, 19, 5, 36, 42, tzinfo=tz.gettz('UTC'))
        self.assertEqual(item.datetime_since, target)

    def test_datetime_since_lxd(self):
        UTC = tz.gettz('UTC')
        item = self.make_status_item(StatusItem.JUJU, '0',
                                     since='30 Nov 2016 09:58:43-05:00')
        target = datetime(2016, 11, 30, 14, 58, 43, tzinfo=UTC)
        self.assertEqual(item.datetime_since.astimezone(UTC), target)

    def test_datetime_since_none(self):
        item = self.make_status_item(StatusItem.JUJU, '0')
        self.assertIsNone(item.datetime_since)

    def test_to_exception_good(self):
        item = self.make_status_item(StatusItem.JUJU, '0', current='idle')
        self.assertIsNone(item.to_exception())

    def test_to_exception_machine_error(self):
        item = self.make_status_item(StatusItem.MACHINE, '0', current='error')
        self.assertIsType(item.to_exception(), MachineError)

    def test_to_exception_provisioning_error(self):
        item = self.make_status_item(StatusItem.MACHINE, '0',
                                     current='provisioning error')
        self.assertIsType(item.to_exception(), ProvisioningError)

    def test_to_exception_stuck_allocating(self):
        item = self.make_status_item(StatusItem.MACHINE, '0',
                                     current='allocating', message='foo')
        with self.assertRaisesRegexp(
                StuckAllocatingError,
                "\\('0', 'Stuck allocating.  Last message: foo'\\)"):
            raise item.to_exception()

    def test_to_exception_allocating_unit(self):
        item = self.make_status_item(StatusItem.JUJU, '0',
                                     current='allocating', message='foo')
        self.assertIs(None, item.to_exception())

    def test_to_exception_app_error(self):
        item = self.make_status_item(StatusItem.APPLICATION, '0',
                                     current='error')
        self.assertIsType(item.to_exception(), AppError)

    def test_to_exception_unit_error(self):
        item = self.make_status_item(StatusItem.WORKLOAD, 'fake/0',
                                     current='error',
                                     message='generic unit error')
        self.assertIsType(item.to_exception(), UnitError)

    def test_to_exception_hook_failed_error(self):
        item = self.make_status_item(StatusItem.WORKLOAD, 'fake/0',
                                     current='error',
                                     message='hook failed: "bad hook"')
        self.assertIsType(item.to_exception(), HookFailedError)

    def test_to_exception_install_error(self):
        item = self.make_status_item(StatusItem.WORKLOAD, 'fake/0',
                                     current='error',
                                     message='hook failed: "install error"')
        self.assertIsType(item.to_exception(), InstallError)

    def make_agent_item_ago(self, minutes):
        now = datetime.utcnow()
        then = now - timedelta(minutes=minutes)
        then_str = then.strftime('%d %b %Y %H:%M:%SZ')
        return self.make_status_item(StatusItem.JUJU, '0', current='error',
                                     message='some error', since=then_str)

    def test_to_exception_agent_error(self):
        item = self.make_agent_item_ago(minutes=3)
        self.assertIsType(item.to_exception(), AgentError)

    def test_to_exception_agent_error_no_since(self):
        item = self.make_status_item(StatusItem.JUJU, '0', current='error')
        self.assertIsType(item.to_exception(), AgentError)

    def test_to_exception_agent_unresolved_error(self):
        item = self.make_agent_item_ago(minutes=6)
        self.assertIsType(item.to_exception(), AgentUnresolvedError)


class TestStatus(FakeHomeTestCase):

    def test_model_name(self):
        status = Status({'model': {'name': 'bar'}}, '')
        self.assertEqual('bar', status.model_name)

    def test_iter_machines_no_containers(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar', 'containers': {'1/lxc/0': {'baz': 'qux'}}}
            },
            'applications': {}}, '')
        self.assertEqual(list(status.iter_machines()),
                         [('1', status.status['machines']['1'])])

    def test_iter_machines_containers(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar', 'containers': {'1/lxc/0': {'baz': 'qux'}}}
            },
            'applications': {}}, '')
        self.assertEqual(list(status.iter_machines(containers=True)), [
            ('1', status.status['machines']['1']),
            ('1/lxc/0', {'baz': 'qux'}),
        ])

    def test__iter_units_in_application(self):
        status = Status({}, '')
        app_status = {
            'units': {'jenkins/1': {'subordinates': {'sub': {'baz': 'qux'}}}}
            }
        expected = [
            ('jenkins/1', {'subordinates': {'sub': {'baz': 'qux'}}}),
            ('sub', {'baz': 'qux'})]
        self.assertItemsEqual(expected,
                              status._iter_units_in_application(app_status))

    def test_agent_items_empty(self):
        status = Status({'machines': {}, 'applications': {}}, '')
        self.assertItemsEqual([], status.agent_items())

    def test_agent_items(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar'}
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'subordinates': {
                                'sub': {'baz': 'qux'}
                            }
                        }
                    }
                }
            }
        }, '')
        expected = [
            ('1', {'foo': 'bar'}),
            ('jenkins/1', {'subordinates': {'sub': {'baz': 'qux'}}}),
            ('sub', {'baz': 'qux'})]
        self.assertItemsEqual(expected, status.agent_items())

    def test_agent_items_containers(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar', 'containers': {
                    '2': {'qux': 'baz'},
                    }}
                },
            'applications': {}
            }, '')
        expected = [
            ('1', {'foo': 'bar', 'containers': {'2': {'qux': 'baz'}}}),
            ('2', {'qux': 'baz'})
            ]
        self.assertItemsEqual(expected, status.agent_items())

    def get_unit_agent_states_data(self):
        status = Status({
            'applications': {
                'jenkins': {
                    'units': {'jenkins/0': {'agent-state': 'good'},
                              'jenkins/1': {'agent-state': 'bad'}},
                    },
                'fakejob': {
                    'life': 'dying',
                    'units': {'fakejob/0': {'agent-state': 'good'}},
                    },
                }
            }, '')
        expected = {
            'good': ['jenkins/0'],
            'bad': ['jenkins/1'],
            'dying': ['fakejob/0'],
            }
        return status, expected

    def test_unit_agent_states_new(self):
        (status, expected) = self.get_unit_agent_states_data()
        actual = status.unit_agent_states()
        self.assertEqual(expected, actual)

    def test_unit_agent_states_existing(self):
        (status, expected) = self.get_unit_agent_states_data()
        actual = defaultdict(list)
        status.unit_agent_states(actual)
        self.assertEqual(expected, actual)

    def test_get_service_count_zero(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
                },
            }, '')
        self.assertEqual(0, status.get_service_count())

    def test_get_service_count(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                    }
                },
                'dummy-sink': {
                    'units': {
                        'dummy-sink/0': {'agent-state': 'started'},
                    }
                },
                'juju-reports': {
                    'units': {
                        'juju-reports/0': {'agent-state': 'pending'},
                    }
                }
            }
        }, '')
        self.assertEqual(3, status.get_service_count())

    def test_get_service_unit_count_zero(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
        }, '')
        self.assertEqual(0, status.get_service_unit_count('jenkins'))

    def test_get_service_unit_count(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'bad'},
                        'jenkins/3': {'agent-state': 'bad'},
                    }
                }
            }
        }, '')
        self.assertEqual(3, status.get_service_unit_count('jenkins'))

    def test_get_unit(self):
        status = Status({
            'machines': {
                '1': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                    }
                },
                'dummy-sink': {
                    'units': {
                        'jenkins/2': {'agent-state': 'started'},
                    }
                },
            }
        }, '')
        self.assertEqual(
            status.get_unit('jenkins/1'), {'agent-state': 'bad'})
        self.assertEqual(
            status.get_unit('jenkins/2'), {'agent-state': 'started'})
        with self.assertRaisesRegexp(KeyError, 'jenkins/3'):
            status.get_unit('jenkins/3')

    def test_service_subordinate_units(self):
        status = Status({
            'machines': {
                '1': {},
            },
            'applications': {
                'ubuntu': {},
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'subordinates': {
                                'chaos-monkey/0': {'agent-state': 'started'},
                            }
                        }
                    }
                },
                'dummy-sink': {
                    'units': {
                        'jenkins/2': {
                            'subordinates': {
                                'chaos-monkey/1': {'agent-state': 'started'}
                            }
                        },
                        'jenkins/3': {
                            'subordinates': {
                                'chaos-monkey/2': {'agent-state': 'started'}
                            }
                        }
                    }
                }
            }
        }, '')
        self.assertItemsEqual(
            status.service_subordinate_units('ubuntu'),
            [])
        self.assertItemsEqual(
            status.service_subordinate_units('jenkins'),
            [('chaos-monkey/0', {'agent-state': 'started'},)])
        self.assertItemsEqual(
            status.service_subordinate_units('dummy-sink'), [
                ('chaos-monkey/1', {'agent-state': 'started'}),
                ('chaos-monkey/2', {'agent-state': 'started'})]
            )

    def test_get_open_ports(self):
        status = Status({
            'machines': {
                '1': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                    }
                },
                'dummy-sink': {
                    'units': {
                        'jenkins/2': {'open-ports': ['42/tcp']},
                    }
                },
            }
        }, '')
        self.assertEqual(status.get_open_ports('jenkins/1'), [])
        self.assertEqual(status.get_open_ports('jenkins/2'), ['42/tcp'])

    def test_agent_states_with_agent_state(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        }, '')
        expected = {
            'good': ['1', 'jenkins/2'],
            'bad': ['jenkins/1'],
            'no-agent': ['2'],
        }
        self.assertEqual(expected, status.agent_states())

    def test_agent_states_with_agent_status(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-status': {'current': 'bad'}},
                        'jenkins/2': {'agent-status': {'current': 'good'}},
                        'jenkins/3': {},
                    }
                }
            }
        }, '')
        expected = {
            'good': ['1', 'jenkins/2'],
            'bad': ['jenkins/1'],
            'no-agent': ['2', 'jenkins/3'],
        }
        self.assertEqual(expected, status.agent_states())

    def test_agent_states_with_juju_status(self):
        status = Status({
            'machines': {
                '1': {'juju-status': {'current': 'good'}},
                '2': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'juju-status': {'current': 'bad'}},
                        'jenkins/2': {'juju-status': {'current': 'good'}},
                        'jenkins/3': {},
                    }
                }
            }
        }, '')
        expected = {
            'good': ['1', 'jenkins/2'],
            'bad': ['jenkins/1'],
            'no-agent': ['2', 'jenkins/3'],
        }
        self.assertEqual(expected, status.agent_states())

    def test_agent_states_with_dying(self):
        status = Status({
            'machines': {},
            'applications': {
                'jenkins': {
                    'life': 'alive',
                    'units': {
                        'jenkins/1': {'juju-status': {'current': 'bad'}},
                        'jenkins/2': {'juju-status': {'current': 'good'}},
                        }
                    },
                'fakejob': {
                    'life': 'dying',
                    'units': {
                        'fakejob/1': {'juju-status': {'current': 'bad'}},
                        'fakejob/2': {'juju-status': {'current': 'good'}},
                        }
                    },
                }
            }, '')
        expected = {
            'good': ['jenkins/2'],
            'bad': ['jenkins/1'],
            'dying': ['fakejob/1', 'fakejob/2'],
            }
        self.assertEqual(expected, status.agent_states())

    def test_check_agents_started_not_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        }, '')
        self.assertEqual(status.agent_states(),
                         status.check_agents_started('env1'))

    def test_check_agents_started_all_started_with_agent_state(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
                '2': {'agent-state': 'started'},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'agent-state': 'started',
                            'subordinates': {
                                'sub1': {
                                    'agent-state': 'started'
                                }
                            }
                        },
                        'jenkins/2': {'agent-state': 'started'},
                    }
                }
            }
        }, '')
        self.assertIsNone(status.check_agents_started('env1'))

    def test_check_agents_started_all_started_with_agent_status(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
                '2': {'agent-state': 'started'},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'agent-status': {'current': 'idle'},
                            'subordinates': {
                                'sub1': {
                                    'agent-status': {'current': 'idle'}
                                }
                            }
                        },
                        'jenkins/2': {'agent-status': {'current': 'idle'}},
                    }
                }
            }
        }, '')
        self.assertIsNone(status.check_agents_started('env1'))

    def test_check_agents_started_dying(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
                '2': {'agent-state': 'started'},
                },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {
                            'agent-status': {'current': 'idle'},
                            'subordinates': {
                                'sub1': {
                                    'agent-status': {'current': 'idle'}
                                    }
                                }
                            },
                        'jenkins/2': {'agent-status': {'current': 'idle'}},
                        },
                    'life': 'dying',
                    }
                }
            }, '')
        self.assertEqual(status.agent_states(),
                         status.check_agents_started('env1'))

    def test_check_agents_started_agent_error(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'any-error'},
            },
            'applications': {}
        }, '')
        with self.assertRaisesRegexp(ErroredUnit,
                                     '1 is in state any-error'):
            status.check_agents_started('env1')

    def do_check_agents_started_agent_state_info_failure(self, failure):
        status = Status({
            'machines': {'0': {
                'agent-state-info': failure}},
            'applications': {},
        }, '')
        with self.assertRaises(ErroredUnit) as e_cxt:
            status.check_agents_started()
        e = e_cxt.exception
        self.assertEqual(
            str(e), '0 is in state {}'.format(failure))
        self.assertEqual(e.unit_name, '0')
        self.assertEqual(e.state, failure)

    def do_check_agents_started_juju_status_failure(self, failure):
        status = Status({
            'machines': {
                '0': {
                    'juju-status': {
                        'current': 'error',
                        'message': failure}
                    },
                }
            }, '')
        with self.assertRaises(ErroredUnit) as e_cxt:
            status.check_agents_started()
        e = e_cxt.exception
        # if message is blank, the failure should reflect the state instead
        if not failure:
            failure = 'error'
        self.assertEqual(
            str(e), '0 is in state {}'.format(failure))
        self.assertEqual(e.unit_name, '0')
        self.assertEqual(e.state, failure)

    def test_check_agents_started_read_juju_status_error(self):
        failures = ['no metadata for "centos7" images in us-east-1 with arches [amd64]',
                    'sending new instance request: GCE operation ' +
                    '"operation-143" failed', '']
        for failure in failures:
            self.do_check_agents_started_juju_status_failure(failure)

    def test_check_agents_started_read_agent_state_info_error(self):
        failures = ['cannot set up groups foobar', 'cannot run instance',
                    'cannot run instances', 'error executing "lxc-start"']
        for failure in failures:
            self.do_check_agents_started_agent_state_info_failure(failure)

    def test_check_agents_started_agent_info_error(self):
        # Sometimes the error is indicated in a special 'agent-state-info'
        # field.
        status = Status({
            'machines': {
                '1': {'agent-state-info': 'any-error'},
            },
            'applications': {}
        }, '')
        with self.assertRaisesRegexp(ErroredUnit,
                                     '1 is in state any-error'):
            status.check_agents_started('env1')

    def test_get_agent_versions_1x(self):
        status = Status({
            'machines': {
                '1': {'agent-version': '1.6.2'},
                '2': {'agent-version': '1.6.1'},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/0': {
                            'agent-version': '1.6.1'},
                        'jenkins/1': {},
                    },
                }
            }
        }, '')
        self.assertEqual({
            '1.6.2': {'1'},
            '1.6.1': {'jenkins/0', '2'},
            'unknown': {'jenkins/1'},
        }, status.get_agent_versions())

    def test_get_agent_versions_2x(self):
        status = Status({
            'machines': {
                '1': {'juju-status': {'version': '1.6.2'}},
                '2': {'juju-status': {'version': '1.6.1'}},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/0': {
                            'juju-status': {'version': '1.6.1'}},
                        'jenkins/1': {},
                    },
                }
            }
        }, '')
        self.assertEqual({
            '1.6.2': {'1'},
            '1.6.1': {'jenkins/0', '2'},
            'unknown': {'jenkins/1'},
        }, status.get_agent_versions())

    def test_iter_new_machines(self):
        old_status = Status({
            'machines': {
                'bar': 'bar_info',
            }
        }, '')
        new_status = Status({
            'machines': {
                'foo': 'foo_info',
                'bar': 'bar_info',
            }
        }, '')
        self.assertItemsEqual(new_status.iter_new_machines(old_status),
                              [('foo', 'foo_info')])

    def test_iter_new_machines_no_containers(self):
        bar_info = {'containers': {'bar/lxd/1': {}}}
        old_status = Status({
            'machines': {
                'bar': bar_info,
            }
        }, '')
        foo_info = {'containers': {'foo/lxd/1': {}}}
        new_status = Status({
            'machines': {
                'foo': foo_info,
                'bar': bar_info,
            }
        }, '')
        self.assertItemsEqual(new_status.iter_new_machines(old_status,
                                                           containers=False),
                              [('foo', foo_info)])

    def test_iter_new_machines_with_containers(self):
        bar_info = {'containers': {'bar/lxd/1': {}}}
        old_status = Status({
            'machines': {
                'bar': bar_info,
            }
        }, '')
        foo_info = {'containers': {'foo/lxd/1': {}}}
        new_status = Status({
            'machines': {
                'foo': foo_info,
                'bar': bar_info,
            }
        }, '')
        self.assertItemsEqual(new_status.iter_new_machines(old_status,
                                                           containers=True),
                              [('foo', foo_info), ('foo/lxd/1', {})])

    def test_get_instance_id(self):
        status = Status({
            'machines': {
                '0': {'instance-id': 'foo-bar'},
                '1': {},
            }
        }, '')
        self.assertEqual(status.get_instance_id('0'), 'foo-bar')
        with self.assertRaises(KeyError):
            status.get_instance_id('1')
        with self.assertRaises(KeyError):
            status.get_instance_id('2')

    def test_get_machine_dns_name(self):
        status = Status({
            'machines': {
                '0': {'dns-name': '255.1.1.0'},
                '1': {},
            }
        }, '')
        self.assertEqual(status.get_machine_dns_name('0'), '255.1.1.0')
        with self.assertRaisesRegexp(KeyError, 'dns-name'):
            status.get_machine_dns_name('1')
        with self.assertRaisesRegexp(KeyError, '2'):
            status.get_machine_dns_name('2')

    def test_from_text(self):
        text = TestModelClient.make_status_yaml(
            'agent-state', 'pending', 'horsefeathers').decode('ascii')
        status = Status.from_text(text)
        self.assertEqual(status.status_text, text)
        self.assertEqual(status.status, {
            'model': {'name': 'foo'},
            'machines': {'0': {'agent-state': 'pending'}},
            'applications': {'jenkins': {'units': {'jenkins/0': {
                'agent-state': 'horsefeathers'}}}}
        })

    def test_iter_units(self):
        started_unit = {'agent-state': 'started'}
        unit_with_subordinates = {
            'agent-state': 'started',
            'subordinates': {
                'ntp/0': started_unit,
                'nrpe/0': started_unit,
            },
        }
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/0': unit_with_subordinates,
                    }
                },
                'application': {
                    'units': {
                        'application/0': started_unit,
                        'application/1': started_unit,
                    }
                },
            }
        }, '')
        expected = [
            ('application/0', started_unit),
            ('application/1', started_unit),
            ('jenkins/0', unit_with_subordinates),
            ('nrpe/0', started_unit),
            ('ntp/0', started_unit),
        ]
        gen = status.iter_units()
        self.assertIsInstance(gen, types.GeneratorType)
        self.assertEqual(expected, list(gen))

    @staticmethod
    def run_iter_status():
        status = Status({
            'machines': {
                '0': {
                    'juju-status': {
                        'current': 'idle',
                        'since': 'DD MM YYYY hh:mm:ss',
                        'version': '2.0.0',
                        },
                    'machine-status': {
                        'current': 'running',
                        'message': 'Running',
                        'since': 'DD MM YYYY hh:mm:ss',
                        },
                    },
                '1': {
                    'juju-status': {
                        'current': 'idle',
                        'since': 'DD MM YYYY hh:mm:ss',
                        'version': '2.0.0',
                        },
                    'machine-status': {
                        'current': 'running',
                        'message': 'Running',
                        'since': 'DD MM YYYY hh:mm:ss',
                        },
                    },
                },
            'applications': {
                'fakejob': {
                    'application-status': {
                        'current': 'idle',
                        'since': 'DD MM YYYY hh:mm:ss',
                        },
                    'units': {
                        'fakejob/0': {
                            'workload-status': {
                                'current': 'maintenance',
                                'message': 'Started',
                                'since': 'DD MM YYYY hh:mm:ss',
                                },
                            'juju-status': {
                                'current': 'idle',
                                'since': 'DD MM YYYY hh:mm:ss',
                                'version': '2.0.0',
                                },
                            },
                        'fakejob/1': {
                            'workload-status': {
                                'current': 'maintenance',
                                'message': 'Started',
                                'since': 'DD MM YYYY hh:mm:ss',
                                },
                            'juju-status': {
                                'current': 'idle',
                                'since': 'DD MM YYYY hh:mm:ss',
                                'version': '2.0.0',
                                },
                            },
                        },
                    }
                },
            }, '')
        for sub_status in status.iter_status():
            yield sub_status

    def test_iter_status_range(self):
        status_set = set([(status_item.item_name, status_item.status_name)
                          for status_item in self.run_iter_status()])
        self.assertEqual({
            ('0', 'juju-status'), ('0', 'machine-status'),
            ('1', 'juju-status'), ('1', 'machine-status'),
            ('fakejob', 'application-status'),
            ('fakejob/0', 'workload-status'), ('fakejob/0', 'juju-status'),
            ('fakejob/1', 'workload-status'), ('fakejob/1', 'juju-status'),
            }, status_set)

    def test_iter_status_data(self):
        min_set = set(['current', 'since'])
        max_set = set(['current', 'message', 'since', 'version'])
        for status_item in self.run_iter_status():
            if 'fakejob' == status_item.item_name:
                self.assertEqual(StatusItem.APPLICATION,
                                 status_item.status_name)
                self.assertEqual({'current': 'idle',
                                  'since': 'DD MM YYYY hh:mm:ss',
                                  }, status_item.status)
            else:
                cur_set = set(status_item.status.keys())
                self.assertTrue(min_set < cur_set)
                self.assertTrue(cur_set < max_set)

    def test_iter_status_container(self):
        status_dict = {'machines': {'0': {
            'containers': {'0/lxd/0': {
                'machine-status': 'foo',
                'juju-status': 'bar',
                }}
            }}}
        status = Status(status_dict, '')
        machine_0_data = status.status['machines']['0']
        container_data = machine_0_data['containers']['0/lxd/0']
        self.assertEqual([
            StatusItem(StatusItem.MACHINE, '0', machine_0_data),
            StatusItem(StatusItem.JUJU, '0', machine_0_data),
            StatusItem(StatusItem.MACHINE, '0/lxd/0', container_data),
            StatusItem(StatusItem.JUJU, '0/lxd/0', container_data),
            ], list(status.iter_status()))

    def test_iter_status_subordinate(self):
        status_dict = {
            'machines': {},
            'applications': {
                'dummy': {
                    'units': {'dummy/0': {
                        'subordinates': {
                            'dummy-sub/0': {}
                            }
                        }},
                    }
                },
            }
        status = Status(status_dict, '')
        dummy_data = status.status['applications']['dummy']
        dummy_0_data = dummy_data['units']['dummy/0']
        dummy_sub_0_data = dummy_0_data['subordinates']['dummy-sub/0']
        self.assertEqual([
            StatusItem(StatusItem.APPLICATION, 'dummy', dummy_data),
            StatusItem(StatusItem.WORKLOAD, 'dummy/0', dummy_0_data),
            StatusItem(StatusItem.JUJU, 'dummy/0', dummy_0_data),
            StatusItem(StatusItem.WORKLOAD, 'dummy-sub/0', dummy_sub_0_data),
            StatusItem(StatusItem.JUJU, 'dummy-sub/0', dummy_sub_0_data),
            ], list(status.iter_status()))

    def test_iter_errors(self):
        status = Status({}, '')
        retval = [
            StatusItem(StatusItem.WORKLOAD, 'job/0', {'current': 'started'}),
            StatusItem(StatusItem.APPLICATION, 'job', {'current': 'started'}),
            StatusItem(StatusItem.MACHINE, '0', {'current': 'error'}),
            ]
        with patch.object(status, 'iter_status', autospec=True,
                          return_value=retval):
            errors = list(status.iter_errors())
        self.assertEqual(len(errors), 1)
        self.assertIsInstance(errors[0], MachineError)
        self.assertEqual(('0', None), errors[0].args)

    def test_iter_errors_ignore_recoverable(self):
        status = Status({}, '')
        retval = [
            StatusItem(StatusItem.WORKLOAD, 'job/0', {'current': 'error'}),
            StatusItem(StatusItem.MACHINE, '0', {'current': 'error'}),
            ]
        with patch.object(status, 'iter_status', autospec=True,
                          return_value=retval):
            errors = list(status.iter_errors(ignore_recoverable=True))
        self.assertEqual(len(errors), 1)
        self.assertIsInstance(errors[0], MachineError)
        self.assertEqual(('0', None), errors[0].args)
        with patch.object(status, 'iter_status', autospec=True,
                          return_value=retval):
            recoverable = list(status.iter_errors())
        self.assertGreater(len(recoverable), len(errors))

    def test_check_for_errors_good(self):
        status = Status({}, '')
        with patch.object(status, 'iter_errors', autospec=True,
                          return_value=[]) as error_mock:
            self.assertEqual([], status.check_for_errors())
        error_mock.assert_called_once_with(False)

    def test_check_for_errors(self):
        status = Status({}, '')
        errors = [MachineError('0'), StatusError('2'), UnitError('1')]
        with patch.object(status, 'iter_errors', autospec=True,
                          return_value=errors) as errors_mock:
            sorted_errors = status.check_for_errors()
        errors_mock.assert_called_once_with(False)
        self.assertEqual(sorted_errors[0].args, ('0',))
        self.assertEqual(sorted_errors[1].args, ('1',))
        self.assertEqual(sorted_errors[2].args, ('2',))

    def test_raise_highest_error(self):
        status = Status({}, '')
        retval = [
            StatusItem(StatusItem.WORKLOAD, 'job/0', {'current': 'error'}),
            StatusItem(StatusItem.MACHINE, '0', {'current': 'error'}),
            ]
        with patch.object(status, 'iter_status', autospec=True,
                          return_value=retval):
            with self.assertRaises(MachineError):
                status.raise_highest_error()

    def test_raise_highest_error_ignore_recoverable(self):
        status = Status({}, '')
        retval = [
            StatusItem(StatusItem.WORKLOAD, 'job/0', {'current': 'error'})]
        with patch.object(status, 'iter_status', autospec=True,
                          return_value=retval):
            status.raise_highest_error(ignore_recoverable=True)
            with self.assertRaises(UnitError):
                status.raise_highest_error(ignore_recoverable=False)

    def test_get_applications_gets_applications(self):
        status = Status({
            'services': {'service': {}},
            'applications': {'application': {}},
            }, '')
        self.assertEqual({'application': {}}, status.get_applications())
