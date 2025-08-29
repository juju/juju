#!/usr/bin/env python3
"""Test charm for integration-testing credential-get on K8s."""

import tempfile

import ops
from kubernetes import client, config


class Charm(ops.CharmBase):
    """Test charm for integration-testing credential-get on K8s."""

    def __init__(self, framework):
        super().__init__(framework)
        framework.observe(self.on.start, self._on_start)
        framework.observe(self.on.get_credentials_action, self._on_get_credentials)
        framework.observe(self.on.hit_k8s_api_default_action, self._on_hit_k8s_api_default)
        framework.observe(
            self.on.hit_k8s_api_credential_get_action, self._on_hit_k8s_api_credential_get
        )

    def _on_start(self, _) -> None:
        self.unit.status = ops.ActiveStatus()

    def _on_get_credentials(self, event: ops.ActionEvent) -> None:
        spec = self.model.get_cloud_spec()
        event.set_results(
            {
                "endpoint": spec.endpoint,
                "token": spec.credential.attributes["Token"] if spec.credential else "<none>",
                "cert": spec.ca_certificates[0],
            }
        )

    def _on_hit_k8s_api_default(self, event: ops.ActionEvent) -> None:
        config.load_incluster_config()
        v1 = client.CoreV1Api()
        ret = v1.list_pod_for_all_namespaces(watch=False)
        pod_names = [item.metadata.name for item in ret.items]
        event.set_results({"pod-names": pod_names})

    def _on_hit_k8s_api_credential_get(self, event: ops.ActionEvent) -> None:
        spec = self.model.get_cloud_spec()
        assert spec.credential

        with tempfile.NamedTemporaryFile("w") as f:
            f.write(spec.ca_certificates[0])
            f.flush()

            configuration = client.Configuration()
            configuration.api_key["authorization"] = spec.credential.attributes["Token"]
            configuration.api_key_prefix["authorization"] = "Bearer"
            configuration.host = spec.endpoint or "<invalid>"
            configuration.ssl_ca_cert = f.name  # type: ignore

            v1 = client.CoreV1Api(client.ApiClient(configuration))
            ret = v1.list_pod_for_all_namespaces(watch=False)
            pod_names = [item.metadata.name for item in ret.items]
            event.set_results({"pod-names": pod_names})


if __name__ == "__main__":
    ops.main(Charm)
