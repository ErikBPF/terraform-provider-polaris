"""Run a command against an owned Polaris fixture in an explicit minikube context."""

import argparse
import base64
import json
import os
import secrets
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


def verify_nodes(payload):
    nodes = payload.get("items", [])
    if not nodes or any(not n.get("metadata", {}).get("labels", {}).get("minikube.k8s.io/name") for n in nodes):
        raise RuntimeError("Refusing fixture deployment: context must contain only minikube nodes")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context", required=True)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    if not args.context.strip():
        parser.error("an explicit nonempty minikube context is required")
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        parser.error("a test command is required after --")
    kubectl = ["kubectl", "--context=" + args.context, "--request-timeout=30s"]

    def kube(arguments, payload=None):
        result = subprocess.run(kubectl + arguments, input=json.dumps(payload) if payload is not None else None, capture_output=True, text=True)
        if result.returncode:
            raise RuntimeError("kubectl " + arguments[0] + " failed (details suppressed to protect fixture credentials)")
        return result.stdout

    verify_nodes(json.loads(kube(["get", "nodes", "-o", "json"])))
    namespace = "polaris-test-" + uuid.uuid4().hex[:16]
    secret = secrets.token_urlsafe(32)
    config = '\n'.join([
        'polaris.realm-context.realms=POLARIS',
        'quarkus.otel.sdk.disabled=true',
        'polaris.readiness.ignore-severe-issues=true',
        'polaris.features."ALLOW_INSECURE_STORAGE_TYPES"=true',
        'polaris.features."SUPPORTED_CATALOG_STORAGE_TYPES"=["FILE"]',
        'polaris.features."ALLOW_NAMESPACE_CUSTOM_LOCATION"=true',
        'polaris.features."ENABLE_SEMANTIC_MODELS"=true',
    ])
    created = False
    forward = None
    with tempfile.TemporaryFile(mode="w+t") as output:
        try:
            # Never apply/adopt a namespace: a name collision must fail before cleanup ownership is acquired.
            kube(["create", "-f", "-"], {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": namespace, "labels": {"app.kubernetes.io/managed-by": "polaris-acceptance"}}})
            created = True
            meta = {"namespace": namespace}
            kube(["create", "-f", "-"], {"apiVersion": "v1", "kind": "Secret", "metadata": {**meta, "name": "bootstrap"}, "data": {"credentials": base64.b64encode(("POLARIS,root," + secret).encode()).decode()}})
            kube(["create", "-f", "-"], {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {**meta, "name": "config"}, "data": {"application.properties": config}})
            kube(["create", "-f", "-"], {"apiVersion": "v1", "kind": "Pod", "metadata": {**meta, "name": "polaris"}, "spec": {
                "automountServiceAccountToken": False, "restartPolicy": "Never",
                "volumes": [{"name": "config", "configMap": {"name": "config"}}],
                "containers": [{"name": "polaris", "image": "docker.io/apache/polaris:1.9.0", "ports": [{"containerPort": 8181}, {"containerPort": 8182}],
                    "env": [{"name": "POLARIS_BOOTSTRAP_CREDENTIALS", "valueFrom": {"secretKeyRef": {"name": "bootstrap", "key": "credentials"}}}, {"name": "JAVA_TOOL_OPTIONS", "value": "-Dquarkus.config.locations=/config/application.properties"}],
                    "volumeMounts": [{"name": "config", "mountPath": "/config", "readOnly": True}],
                    "resources": {"requests": {"cpu": "500m", "memory": "512Mi"}, "limits": {"cpu": "2", "memory": "2Gi"}},
                    "readinessProbe": {"httpGet": {"path": "/q/health/ready", "port": 8182}, "initialDelaySeconds": 3, "periodSeconds": 2}}]}})
            kube(["-n", namespace, "wait", "--for=condition=Ready", "pod/polaris", "--timeout=180s"])
            forward = subprocess.Popen(kubectl + ["-n", namespace, "port-forward", "--address=127.0.0.1", "pod/polaris", ":8181"], stdout=output, stderr=output, text=True)
            deadline = time.monotonic() + 30
            base = None
            while time.monotonic() < deadline:
                output.seek(0)
                for line in output.read().splitlines():
                    if line.startswith("Forwarding from 127.0.0.1:"):
                        base = "http://" + line.split()[2]
                        break
                if base:
                    break
                if forward.poll() is not None:
                    raise RuntimeError("Fixture port-forward exited")
                time.sleep(0.2)
            if not base:
                raise RuntimeError("Fixture port-forward timed out")
            data = urllib.parse.urlencode({"grant_type": "client_credentials", "client_id": "root", "client_secret": secret, "scope": "PRINCIPAL_ROLE:ALL"}).encode()
            try:
                with urllib.request.urlopen(urllib.request.Request(base + "/api/catalog/v1/oauth/tokens", data=data), timeout=20) as response:
                    token = json.load(response)["access_token"]
            except (urllib.error.URLError, KeyError, ValueError):
                raise RuntimeError("Fixture authentication failed") from None
            env = {k: v for k, v in os.environ.items() if not k.startswith(("POLARIS_", "TF_LOG", "TF_CLI_ARGS"))}
            env.update({"TF_ACC": "1", "TF_IN_AUTOMATION": "true", "POLARIS_ACCEPTANCE_FIXTURE": "owned", "POLARIS_TEST_BACKEND": "minikube", "POLARIS_BASE_URL": base, "POLARIS_REALM": "POLARIS", "POLARIS_TOKEN": token, "POLARIS_TEST_ROOT_SECRET": secret})
            print("Fixture:", args.context, namespace, flush=True)
            result = subprocess.run(command, env=env, check=False)
            if result.returncode:
                raise RuntimeError("Fixture test command failed: exit " + str(result.returncode))
        finally:
            original_error = sys.exc_info()[0] is not None
            if forward:
                forward.terminate()
                try:
                    forward.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    forward.kill()
                    forward.wait(timeout=10)
            if created:
                try:
                    kube(["delete", "namespace", namespace, "--wait=true", "--timeout=120s"])
                    if json.loads(kube(["get", "namespaces", "--field-selector=metadata.name=" + namespace, "-o", "json"]))["items"]:
                        raise RuntimeError("Owned fixture namespace remains")
                except RuntimeError:
                    if not original_error:
                        raise
                    print("Fixture cleanup incomplete: " + namespace, file=sys.stderr)
                else:
                    print("PASS: owned minikube namespace absent", flush=True)


if __name__ == "__main__":
    main()
