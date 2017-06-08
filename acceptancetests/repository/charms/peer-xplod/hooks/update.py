#!/usr/bin/env python

import datetime
import errno
import json
import os
import subprocess
import time

defaultMaximum = 1000
unitName = os.environ.get('JUJU_UNIT_NAME')

def run_json(cmd):
    """run a command, treat the output as JSON and return the parsed result."""
    try:
        out = subprocess.check_output(cmd + ['--format=json'])
    except subprocess.CalledProcessError as e:
        raise
    return json.loads(out)


def juju_log(msg):
    print msg
    #subprocess.check_call(["juju-log", msg])


STATE_MAINTENANCE = "maintenance"
STATE_BLOCKED = "blocked"
STATE_WAITING = "waiting"
STATE_ACTIVE = "active"

def set_unit_status(state, msg=""):
    """Set the state of this unit and give a message.

    For compatibility, this is a no-op if 'status-set' doesn't exist.
    """
    try:
        subprocess.check_call(["status-set", state, msg])
    except os.Error as e:
        if e.errno == errno.ENOENT:
            return
        raise


def my_relation_id():
    """xplod is a peer relation, so we know there is only one relation-id."""
    relation_ids = run_json(["relation-ids", "xplod"])
    if len(relation_ids) == 0:
        # This can happen in 'config-changed' before we join the peer relation
        return None
    assert len(relation_ids) == 1
    return relation_ids[0]

def my_info(relation_id=None):
    cmd = ["relation-get"]
    if relation_id is not None:
        cmd.extend(["-r", relation_id])
    cmd.extend(["-", unitName])
    return run_json(cmd)

def my_config():
    return run_json(["config-get"])

def remote_info():
    return run_json(["relation-get", "-"])

def relation_set(relation_id=None, **kwargs):
    args = []
    for key in sorted(kwargs.keys()):
        args.append('%s=%s' % (key, kwargs[key]))
    cmd = ["relation-set"]
    if relation_id is not None:
        cmd.extend(["-r", relation_id])
    subprocess.check_call(cmd + args)


def check_if_started(info, relation_id=None):
    """Check if info contains a 'started' field, and if not, create it."""
    if 'started' in info:
        return
    now = time.time()
    utc_date = datetime.datetime.utcfromtimestamp(now)
    relation_set(relation_id=relation_id,
        started=utc_date.isoformat(),
        started_timestamp=("%d"% (now,)),
    )


def determine_new_value(info, other):
    my_v = int(info.get("v", 0))
    other_v = int(other.get("v", 0))
    if my_v > other_v:
        juju_log("local v greater %s > %s" % (my_v, other_v))
        v = my_v
    else:
        v = other_v
    return v + 1


def determine_maximum(config):
    return int(config.get('maximum', defaultMaximum))


def check_set_stopped(info, v, relation_id=None):
    # Check if stopped has a value in it
    if "stopped" in info and info["stopped"]:
        return
    now = time.time()
    utc_date = datetime.datetime.utcfromtimestamp(now)
    total = 0
    if 'started_timestamp' in info:
        started_timestamp = int(info['started_timestamp'])
        total = now - started_timestamp
    relation_set(relation_id=relation_id,
        stopped=utc_date.isoformat(),
        stopped_timestamp=("%d"% (now,)),
        total_time=("%d"% (total,)),
        v=v,
    )


def update_locally():
    """Not run in a relation context, so we have to determine more information."""
    relation_id = my_relation_id()
    if relation_id is None:
        juju_log("no relation-id, no peers visible")
        set_unit_status(STATE_WAITING, "waiting for peers")
        return

    info = my_info(relation_id=relation_id)
    check_if_started(info, relation_id=relation_id)
    config = my_config()
    maximum = determine_maximum(config)
    v = int(info.get("v", 0)) + 1
    check_set_v(v, maximum, info, relation_id=relation_id)


def check_set_v(v, maximum, info, relation_id=None):
    if maximum <= 0 or v <= maximum:
        juju_log("setting v %s.v=%s" % (unitName, v))
        relation_set(relation_id=relation_id, v=v, stopped="")
        set_unit_status(STATE_ACTIVE)
    else:
        juju_log("maximum (%s) exceeded %s" % (maximum, v))
        check_set_stopped(info, maximum, relation_id=relation_id)
        set_unit_status(STATE_WAITING, "reached maximum count %d" % (maximum,))


def update_value():
    info = my_info()
    check_if_started(info)
    config = my_config()
    other = remote_info()
    maximum = determine_maximum(config)
    v = determine_new_value(info, other)
    check_set_v(v, maximum, info)
