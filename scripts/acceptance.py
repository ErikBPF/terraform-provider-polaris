"""Run provider acceptance against one owned, disposable loopback Polaris container."""

import json
import os
import secrets
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


def run(args, *, env=None):
    result = subprocess.run(args, env=env, capture_output=True, text=True, check=False)
    if result.returncode:
        raise RuntimeError(f"{args[0]} failed with exit {result.returncode}")
    return result.stdout.strip()


def main():
    engine = (
        os.environ.get("CONTAINER_ENGINE")
        or shutil.which("podman")
        or shutil.which("docker")
    )
    if not engine:
        raise RuntimeError("Docker or Podman is required")
    name = "polaris-provider-test-" + uuid.uuid4().hex
    secret = secrets.token_urlsafe(32)
    try:
        run(
            [
                engine,
                "run",
                "--detach",
                "--name",
                name,
                "--publish",
                "127.0.0.1::8181",
                "--publish",
                "127.0.0.1::8182",
                "--env",
                "POLARIS_BOOTSTRAP_CREDENTIALS=POLARIS,root," + secret,
                "--env",
                "polaris.realm-context.realms=POLARIS",
                "--env",
                "quarkus.otel.sdk.disabled=true",
                "--env",
                "polaris.readiness.ignore-severe-issues=true",
                "--env",
                'polaris.features."ALLOW_INSECURE_STORAGE_TYPES"=true',
                "--env",
                'polaris.features."SUPPORTED_CATALOG_STORAGE_TYPES"=["FILE"]',
                "--env", 'polaris.features."ENABLE_SEMANTIC_MODELS"=true',
                "--env", 'polaris.features."ALLOW_NAMESPACE_CUSTOM_LOCATION"=true',
                "docker.io/apache/polaris:1.9.0",
            ]
        )
        address = run([engine, "port", name, "8181/tcp"])
        health = run([engine, "port", name, "8182/tcp"])
        if not address.startswith("127.0.0.1:") or not health.startswith("127.0.0.1:"):
            raise RuntimeError("Fixture must bind exclusively to loopback")
        deadline = time.monotonic() + 120
        while True:
            try:
                with urllib.request.urlopen(
                    "http://" + health + "/q/health/ready", timeout=2
                ) as response:
                    if json.load(response)["status"] == "UP":
                        break
            except (OSError, ValueError, KeyError):
                pass
            if (
                run([engine, "inspect", "--format", "{{.State.Running}}", name])
                != "true"
            ):
                raise RuntimeError("Disposable Polaris exited during startup")
            if time.monotonic() > deadline:
                raise RuntimeError("Disposable Polaris readiness timed out")
            time.sleep(2)
        form = urllib.parse.urlencode(
            {
                "grant_type": "client_credentials",
                "client_id": "root",
                "client_secret": secret,
                "scope": "PRINCIPAL_ROLE:ALL",
            }
        ).encode()
        req = urllib.request.Request(
            "http://" + address + "/api/catalog/v1/oauth/tokens", data=form
        )
        try:
            with urllib.request.urlopen(req, timeout=20) as response:
                token = json.load(response)["access_token"]
        except urllib.error.HTTPError as error:
            raise RuntimeError(
                f"Fixture token acquisition failed: HTTP {error.code}"
            ) from None
        env = {
            k: v
            for k, v in os.environ.items()
            if not k.startswith(("POLARIS_", "TF_LOG", "TF_CLI_ARGS"))
        }
        env.update(
            {
                "TF_ACC": "1",
                "TF_IN_AUTOMATION": "true",
                "POLARIS_ACCEPTANCE_FIXTURE": "owned",
                "POLARIS_BASE_URL": "http://" + address,
                "POLARIS_REALM": "POLARIS",
                "POLARIS_TOKEN": token,
            }
        )
        result = subprocess.run(
            [
                os.environ.get("GO", "go"),
                "test",
                "./polaris",
                "-run",
                "TestAcc",
                "-count=1",
                "-timeout=15m",
            ],
            env=env,
            check=False,
        )
        if result.returncode:
            raise RuntimeError(f"Provider acceptance failed: exit {result.returncode}")
        print(
            "PASS: IAM and asset lifecycles/imports, no-op, property updates, drift repair and nonpurging unregister"
        )
    finally:
        original_error = sys.exc_info()[0] is not None
        try:
            subprocess.run(
                [engine, "rm", "--force", name],
                capture_output=True,
                text=True,
                check=False,
            )
            remaining = run(
                [
                    engine,
                    "ps",
                    "--all",
                    "--filter",
                    "name=" + name,
                    "--format",
                    "{{.Names}}",
                ]
            )
            if name in remaining.splitlines():
                raise RuntimeError("Owned fixture cleanup incomplete")
        except (OSError, RuntimeError):
            if not original_error:
                raise RuntimeError(
                    "Cannot verify owned fixture cleanup: " + name
                ) from None
            print("Owned fixture cleanup incomplete: " + name, file=sys.stderr)
        else:
            print("PASS: owned fixture container absent")


if __name__ == "__main__":
    main()
