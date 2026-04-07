FROM public.ecr.aws/ubuntu/ubuntu:24.04
ARG TARGETOS
ARG TARGETARCH

<<<<<<< HEAD
# Add the syslog user for audit logging and juju rootless controller.
# 170 uid/gid must be updated here and in internal/provider/kubernetes/constants/constants.go
RUN useradd --system --no-create-home --shell /usr/sbin/nologin syslog \
    && groupadd --gid 170 juju \
    && useradd --uid 170 --gid 170 --no-create-home --shell /usr/bin/bash juju \
    && apt-get update \
    && apt-get install -y --no-install-recommends vim less ca-certificates \
    && apt-get remove sudo --purge \
    && apt-get autoremove --purge \
=======
# Add the syslog user for audit logging.
RUN useradd --system --no-create-home --shell /usr/sbin/nologin syslog
# Add the juju and sjuju user for rootless agents.
# 170 and 171 uid/gid must be updated here and in caas/kubernetes/provider/constants/constants.go
RUN groupadd --gid 170 juju
RUN useradd --uid 170 --gid 170 --no-create-home --shell /usr/bin/bash juju
RUN groupadd --gid 171 sjuju
RUN useradd --uid 171 --gid 171 --no-create-home --shell /usr/bin/bash sjuju
RUN mkdir -p /etc/sudoers.d && echo "sjuju ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/sjuju

# Some Python dependencies.
RUN DEBIAN_FRONTEND=noninteractive apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    sudo \
    python3-yaml \
    python3-pip \
    python3-setuptools \
    # for debug-hooks.
    tmux byobu \
    curl iproute2 \
>>>>>>> 3.6
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

WORKDIR /var/lib/juju
COPY ${TARGETOS}_${TARGETARCH}/bin/jujud \
     ${TARGETOS}_${TARGETARCH}/bin/jujuc \
     ${TARGETOS}_${TARGETARCH}/bin/containeragent \
     ${TARGETOS}_${TARGETARCH}/bin/pebble /opt/

ENTRYPOINT ["sh", "-c"]
