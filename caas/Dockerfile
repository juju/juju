ARG BASE_IMAGE
FROM $BASE_IMAGE

# Add the syslog user for audit logging.
RUN useradd --system -M syslog
RUN usermod -s /usr/sbin/nologin syslog

# Some Python dependencies.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    python3-yaml \
    python3-pip \
    python3-distutils \
    # below apt dependencies are required by controller pod.
    iproute2 \
    curl \
    && pip3 install --upgrade pip setuptools \
    && rm -rf /var/lib/apt/lists/* \
    && rm -rf /root/.cache

# Install the standard charm dependencies.
ENV WHEELHOUSE=/tmp/wheelhouse
ENV PIP_WHEEL_DIR=/tmp/wheelhouse
ENV PIP_FIND_LINKS=/tmp/wheelhouse

COPY requirements.txt /tmp/wheelhouse/requirements.txt
RUN pip3 install -r /tmp/wheelhouse/requirements.txt

WORKDIR /var/lib/juju
COPY jujuc /opt/
# copy Juju executables.
# in-pod agent binary will be "k8sagent".
ARG AGENT_BINARY_NAME=jujud
COPY $AGENT_BINARY_NAME /opt/

ENTRYPOINT ["sh", "-c"]
