def config_status_message(config):
    parts = []
    for key in sorted(config.keys()):
        parts.append("%s=%s" % (key, config[key]))
    config_values = ", ".join(parts) if parts else "no config"
    return "config changed :" + config_values
