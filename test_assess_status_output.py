__metaclass__ = type

from status import (
    ErrNoStatus,
    StatusYamlParser,
    StatusJsonParser,
    StatusTabularParser
)

from unittest import TestCase

SAMPLE_YAML_OUTPUT = """environment: bogusec2
machines:
  "0":
    agent-state: started
    agent-version: 1.25-alpha1
    dns-name: 54.82.51.4
    instance-id: i-c0dadd10
    instance-state: running
    series: trusty
    hardware: arch=amd64 cpu-cores=1 cpu-power=100 mem=1740M """ \
    """root-disk=8192M availability-zone=us-east-1c
    state-server-member-status: has-vote
  "1":
    agent-state: started
    agent-version: 1.25-alpha1
    dns-name: 54.162.95.230
    instance-id: i-39280ac6
    instance-state: running
    series: trusty
    hardware: arch=amd64 cpu-cores=1 cpu-power=100 mem=1740M """\
    """root-disk=8192M availability-zone=us-east-1d
services:
  statusstresser:
    charm: local:trusty/statusstresser-1
    exposed: false
    service-status:
      current: active
      message: called in config-changed hook
      since: 12 Jun 2015 13:15:25-03:00
    units:
      statusstresser/0:
        workload-status:
          current: active
          message: called in config-changed hook
          since: 12 Jun 2015 13:15:25-03:00
        agent-status:
          current: idle
          since: 12 Jun 2015 14:33:08-03:00
          version: 1.25-alpha1
        agent-state: started
        agent-version: 1.25-alpha1
        machine: "1"
        public-address: 54.162.95.230
"""
# This could be replaced by a json.dumps({}) but the text is kept to
# make this test about status output as accurate as possible.
SAMPLE_JSON_OUTPUT = \
    """{"environment":"perritoec2","machines":{"0":{"agent-state":"started","""\
    """"agent-version":"1.25-alpha1","dns-name":"54.82.51.4","""\
    """"instance-id":"i-c0dadd10","instance-state":"running","""\
    """"series":"trusty","hardware":"arch=amd64 cpu-cores=1 cpu-power=100 """\
    """mem=1740M root-disk=8192M availability-zone=us-east-1c","""\
    """"state-server-member-status":"has-vote"},"1":{"agent-state":"""\
    """"started","agent-version":"1.25-alpha1","dns-name":"54.162.95.230","""\
    """"instance-id":"i-a7a2b377","instance-state":"running","""\
    """"series":"trusty","hardware":"arch=amd64 cpu-cores=1 cpu-power=300 """\
    """mem=3840M root-disk=8192M availability-zone=us-east-1c"}},"""\
    """"services":{"statusstresser":{"charm": """\
    """"local:trusty/statusstresser-1","exposed":false,"service-status":"""\
    """{"current":"active","message":"called in config-changed hook","""\
    """"since":"15 Jun 2015 20:56:29-03:00"},"""\
    """"units":{"statusstresser/0":{"workload-status":"""\
    """{"current":"active","message":"called in config-changed hook","""\
    """"since":"15 Jun 2015 20:56:29-03:00"},"agent-status":"""\
    """{"current":"idle","since":"15 Jun 2015 20:56:41-03:00","""\
    """"version":"1.25-alpha1"},"agent-state":"started","agent-version":"""\
    """"1.25-alpha1","machine":"1","public-address":"54.162.95.230"}}}}}"""

SAMPLE_TABULAR_OUTPUT = """[Services]
NAME           STATUS EXPOSED CHARM
statusstresser active false   local:trusty/statusstresser-1

[Units]
ID               WORKLOAD-STATE AGENT-STATE VERSION     MACHINE PORTS """\
"""PUBLIC-ADDRESS MESSAGE
statusstresser/0 active         idle        1.25-alpha1 1             """\
"""54.162.95.230  called in config-changed hook

[Machines]
ID         STATE   VERSION     DNS            INS-ID     SERIES HARDWARE
0          started 1.25-alpha1 54.82.51.4     i-c0dadd10 trusty arch=amd64 """\
    """cpu-cores=1 cpu-power=100 mem=1740M root-disk=8192M """\
    """availability-zone=us-east-1c
1          started 1.25-alpha1 54.162.95.230  i-39280ac6 trusty arch=amd64 """\
    """cpu-cores=1 cpu-power=100 mem=1740M root-disk=8192M """\
    """availability-zone=us-east-1d"""


class ReducedTestStatus:

    def test_assert_machine_ids(self):
        self.parser.assert_machines_ids(["0", "1"])

    def test_assert_machine_ids_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machines_ids(["0", "1", "2"])

    def test_assert_machine_len(self):
        self.parser.assert_machines_len(2)

    def test_assert_machine_len_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machines_len(3)

    def test_machine_agent_state_valid(self):
        self.parser.assert_machine_agent_state("0", "started")

    def test_machine_agent_state_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_agent_state("0", "stopped")

    def test_machine_agent_state_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_agent_state("3", "stopped")

    def test_assert_machine_agent_version(self):
        self.parser.assert_machine_agent_version("0", "1.25-alpha1")

    def test_assert_machine_agent_version_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_agent_version("0", "1.25-alpha2")

    def test_assert_machine_agent_version_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_agent_version("5", "1.25-alpha1")

    def test_assert_machine_dns_name(self):
        self.parser.assert_machine_dns_name("0", "54.82.51.4")

    def test_assert_machine_dns_name_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_dns_name("0", "54.82.51.5")

    def test_assert_machine_dns_name_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_dns_name("3", "54.82.51.4")

    def test_assert_machine_instance_id(self):
        self.parser.assert_machine_instance_id("0", "i-c0dadd10")

    def test_assert_machine_instance_id_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_instance_id("0", "i-c0dadd11")

    def test_assert_machine_instance_id_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_instance_id("3", "i-c0dadd10")

    def test_assert_machine_series(self):
        self.parser.assert_machine_series("0", "trusty")

    def test_assert_machine_series_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_series("0", "utopic")

    def test_assert_machine_series_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_series("3", "trusty")

    def test_assert_machine_hardware(self):
        self.parser.assert_machine_hardware("0", "arch=amd64 cpu-cores=1 "
                                                 "cpu-power=100 mem=1740M "
                                                 "root-disk=8192M "
                                                 "availability-zone="
                                                 "us-east-1c")

    def test_assert_machine_hardware_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_hardware("0", "arch=arm cpu-cores=1 "
                                                     "cpu-power=100 mem=1740M "
                                                     "root-disk=8192M "
                                                     "availability-zone="
                                                     "us-east-1c")

    def test_assert_machine_hardware_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_hardware("3", "arch=amd64 cpu-cores=1 "
                                                     "cpu-power=100 mem=1740M "
                                                     "root-disk=8192M "
                                                     "availability-zone="
                                                     "us-east-1c")

    def test_assert_service_charm(self):
        self.parser.assert_service_charm("statusstresser",
                                         "local:trusty/statusstresser-1")

    def test_assert_service_charm_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_charm("statusstresser",
                                             "local:trusty/statusstresser-2")

    def test_assert_service_charm_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_charm("statusrelaxer",
                                             "local:trusty/statusstresser-1")

    def test_assert_service_exposed(self):
        self.parser.assert_service_exposed("statusstresser", False)

    def test_assert_service_exposed_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_exposed("statusstresser", True)

    def test_assert_service_exposed_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_exposed("statusrelaxer", False)

    def test_assert_unit_public_address(self):
        self.parser.assert_unit_public_address("statusstresser/0",
                                               "54.162.95.230")

    def test_assert_unit_public_address_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_public_address("statusstresser/0",
                                                   "54.162.95.231")

    def test_assert_unit_public_address_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_public_address("statusrelaxer/0",
                                                   "54.162.95.230")


class BaseTestStatus(ReducedTestStatus):

    def test_assert_machine_member_status(self):
        self.parser.assert_machine_member_status("0", "has-vote")

    def test_assert_machine_member_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_member_status("0", "not-voting")

    def test_assert_machine_member_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_machine_member_status("3", "has-vote")

    def test_assert_service_service_status(self):
        self.parser.assert_service_service_status("statusstresser",
                                                  {"current": "active",
                                                   "message": "called in "
                                                   "config-changed hook"})

    def test_assert_service_service_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_service_status("statusstresser",
                                                      {"current": "active",
                                                       "message": "another "
                                                       "message"})

    def test_assert_service_service_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_service_status("statusrelaxer",
                                                      {"current": "active",
                                                       "message": "called in "
                                                       "config-changed hook"})

    def test_assert_unit_workload_status(self):
        self.parser.assert_unit_workload_status("statusstresser/0",
                                                {"current": "active",
                                                 "message": "called in "
                                                 "config-changed hook"})

    def test_assert_unit_workload_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_workload_status("statusstresser/0",
                                                    {"current": "active",
                                                     "message": "another "
                                                     "message"})

    def test_assert_unit_workload_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_workload_status("statusrelaxer/0",
                                                    {"current": "active",
                                                     "message": "called in "
                                                     "config-changed hook"})

    def test_assert_unit_agent_status(self):
        self.parser.assert_unit_agent_status("statusstresser/0",
                                             {"current": "idle",
                                              "message": ""})

    def test_assert_unit_agent_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_status("statusstresser/0",
                                                 {"current": "idle",
                                                  "message": "an unexpected "
                                                  "message"})

    def test_assert_unit_agent_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_status("statusrelaxer/0",
                                                 {"current": "idle",
                                                  "message": ""})

    def test_assert_unit_agent_state(self):
        self.parser.assert_unit_agent_state("statusstresser/0", "started")

    def test_assert_unit_agent_state_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_state("statusstresser/0", "stopped")

    def test_assert_unit_agent_state_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_state("statusrelaxer/0", "started")

    def test_assert_unit_agent_version(self):
        self.parser.assert_unit_agent_version("statusstresser/0",
                                              "1.25-alpha1")

    def test_assert_unit_agent_version_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_version("statusstresser/0",
                                                  "1.25-alpha2")

    def test_assert_unit_agent_version_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_version("statusrelaxer/0",
                                                  "1.25-alpha1")

    def test_assert_unit_machine(self):
        self.parser.assert_unit_machine("statusstresser/0", "1")

    def test_assert_unit_machine_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_machine("statusstresser/0", "2")

    def test_assert_unit_machine_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_machine("statusrelaxer/0", "1")


class TestStatusForYaml(TestCase, BaseTestStatus):

    def test_empty_yaml_fails(self):
        with self.assertRaises(ErrNoStatus):
            StatusYamlParser(yaml="")

    def setUp(self):
        self.parser = StatusYamlParser(yaml=SAMPLE_YAML_OUTPUT)


class TestStatusForJson(TestCase, BaseTestStatus):

    def test_empty_json_fails(self):
        with self.assertRaises(ErrNoStatus):
            StatusJsonParser(json_text="")

    def setUp(self):
        self.parser = StatusJsonParser(json_text=SAMPLE_JSON_OUTPUT)


class TestStatusTabular(TestCase, ReducedTestStatus):

    def test_empty_tabular_fails(self):
        with self.assertRaises(ErrNoStatus):
            StatusTabularParser("")

    def setUp(self):
        self.parser = StatusTabularParser(tabular_text=SAMPLE_TABULAR_OUTPUT)

    def test_assert_service_service_status(self):
        self.parser.assert_service_service_status("statusstresser",
                                                  {"current": "active",
                                                   "message": ""})

    def test_assert_service_service_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_service_status("statusstresser",
                                                      {"current": "active",
                                                       "message": "another "
                                                       "message"})

    def test_assert_service_service_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_service_service_status("statusrelaxer",
                                                      {"current": "active",
                                                       "message": "called in "
                                                       "config-changed hook"})

    def test_assert_unit_workload_status(self):
        self.parser.assert_unit_workload_status("statusstresser/0",
                                                {"current": "active",
                                                 "message": "called in "
                                                 "config-changed hook"})

    def test_assert_unit_workload_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_workload_status("statusstresser/0",
                                                    {"current": "active",
                                                     "message": "another "
                                                     "message"})

    def test_assert_unit_workload_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_workload_status("statusrelaxer/0",
                                                    {"current": "active",
                                                     "message": "called in "
                                                     "config-changed hook"})

    def test_assert_unit_agent_status(self):
        self.parser.assert_unit_agent_status("statusstresser/0",
                                             {"current": "idle",
                                              "message": ""})

    def test_assert_unit_agent_status_failed(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_status("statusstresser/0",
                                                 {"current": "idle",
                                                  "message": "an unexpected "
                                                  "message"})

    def test_assert_unit_agent_status_error(self):
        with self.assertRaises(AssertionError):
            self.parser.assert_unit_agent_status("statusrelaxer/0",
                                                 {"current": "idle",
                                                  "message": ""})
