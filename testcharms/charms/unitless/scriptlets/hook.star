load("status.star", "other_file_func")

def init():
    juju.observe("config_changed", on_config_changed)
    juju.observe("relation_created", on_relation_created)
    juju.observe("relation_changed", on_relation_changed)
    juju.observe("relation_broken", on_relation_broken)

def on_config_changed(event):
    print("config changed")
    other_file_func()

def on_relation_created(event):
    print("relation created")

def on_relation_changed(event):
    print("relation changed")

def on_relation_broken(event):
    print("relation broken")
