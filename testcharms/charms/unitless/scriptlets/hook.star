load("status.star", "config_status_message")

def init():
    juju.observe("config_changed", on_config_changed)
    juju.observe("relation_created", on_relation_created)
    juju.observe("relation_joined", on_relation_joined)
    juju.observe("relation_changed", on_relation_changed)
    juju.observe("relation_departed", on_relation_departed)
    juju.observe("relation_broken", on_relation_broken)

def on_config_changed(event):
    juju.set_status("active", message = config_status_message(event.config))

def on_relation_created(event):
    juju.set_status("active", message = "relation created")

def on_relation_joined(event):
    juju.set_status("active", message = "related")
    juju.set_state("controller-uuid", event.controller_uuid)
    juju.set_state("model-name", event.model_name)

def on_relation_changed(event):
    juju.set_status("active", message = "relation changed")

def on_relation_departed(event):
    juju.set_status("waiting", message = "relation departed")

def on_relation_broken(event):
    juju.set_status("waiting", message = "relation broken")
