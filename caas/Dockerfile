FROM public.ecr.aws/ubuntu/ubuntu:24.04
ARG TARGETOS
ARG TARGETARCH

# Add the syslog user for audit logging and juju rootless controller.
# 170 uid/gid must be updated here and in internal/provider/kubernetes/constants/constants.go
RUN useradd --system --no-create-home --shell /usr/sbin/nologin syslog \
    && groupadd --gid 170 juju \
    && useradd --uid 170 --gid 170 --no-create-home --shell /usr/bin/bash juju \
    && apt-get update \
    && apt-get install -y --no-install-recommends vim less ca-certificates \
    && apt-get remove sudo --purge \
    && apt-get autoremove --purge \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

WORKDIR /var/lib/juju
COPY ${TARGETOS}_${TARGETARCH}/bin/jujuagentd \
     ${TARGETOS}_${TARGETARCH}/bin/jujuc \
     ${TARGETOS}_${TARGETARCH}/bin/containeragent \
     ${TARGETOS}_${TARGETARCH}/bin/pebble /opt/

# Compatibility shim for upgrades from controllers bootstrapped before the
# agent binary was renamed from `jujud` to `jujuagentd`. The pre-rename
# StatefulSet pod template bakes Args that reference `/opt/jujud` and
# `<tools-dir>/jujud`. Kubernetes upgrade only swaps the container image,
# leaving those Args intact, so without this symlink the old `cp /opt/jujud`
# in the Args fails on the new image and the old binary on the PVC keeps
# running. With the symlink, the legacy paths resolve to the new binary.
RUN ln -s jujuagentd /opt/jujud

ENTRYPOINT ["sh", "-c"]
