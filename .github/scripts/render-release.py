"""Render Kubernetes release assets using the digest from each image job."""

import argparse
import json
import re
import subprocess
from pathlib import Path
from tempfile import TemporaryDirectory

APP_IMAGES = {
    "controlplane",
    "runtime-controller",
    "dashboard",
    "mcp",
    "inference-gateway",
    "egress-gateway",
    "credential-broker",
}
WORKSPACE_IMAGES = {"workspace", "workspace-hermes"}
BASE_IMAGE_NAMESPACE = "ghcr.io/robrotheram/harbormaster"


def image_inventory(digest_dir, namespace):
    images = {}
    for path in sorted(digest_dir.glob("*.json")):
        metadata = json.loads(path.read_text())
        name = metadata["name"]
        digest = metadata["digest"]
        if name in images or not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
            raise ValueError(f"Invalid or duplicate image metadata in {path}")
        images[name] = f"{namespace}/{name}@{digest}"
    if images.keys() != APP_IMAGES | WORKSPACE_IMAGES:
        raise ValueError(f"Unexpected image set: {', '.join(images)}")
    return images


def render(kubernetes_dir, kustomize, images, version, output, hermes=False):
    workspace = images["workspace-hermes" if hermes else "workspace"]
    def env_patch(service, name, value):
        return {
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {"name": service, "namespace": "harbormaster"},
            "spec": {
                "template": {
                    "spec": {
                        "containers": [
                            {"name": service, "env": [{"name": name, "value": value}]}
                        ]
                    }
                }
            },
        }

    kustomization = {
        "apiVersion": "kustomize.config.k8s.io/v1beta1",
        "kind": "Kustomization",
        "resources": ["../base"],
        "images": [
            {
                "name": f"{BASE_IMAGE_NAMESPACE}/{name}",
                "newName": images[name].split("@")[0],
                "digest": images[name].split("@")[1],
            }
            for name in sorted(APP_IMAGES)
        ],
        "patches": [
            {"patch": json.dumps(env_patch("runtime-controller", "WORKSPACE_IMAGE", workspace))},
            {"patch": json.dumps(env_patch("controlplane", "HARBORMASTER_VERSION", version))},
        ],
    }
    if hermes:
        kustomization["components"] = ["../components/hermes"]
    with TemporaryDirectory(prefix=".release-", dir=kubernetes_dir) as temp_dir:
        wrapper = Path(temp_dir)
        (wrapper / "kustomization.yaml").write_text(json.dumps(kustomization))
        content = subprocess.check_output([kustomize, "build", wrapper], text=True)
    expected_images = {images[name] for name in APP_IMAGES}
    rendered_images = set(re.findall(r"^\s+image: (ghcr\.io/\S+)$", content, re.M))
    if rendered_images != expected_images:
        raise ValueError(f"Rendered {output.name} has unexpected application image references")
    if f"value: {workspace}\n" not in content:
        raise ValueError(f"Rendered {output.name} is missing the workspace digest")
    if f"value: {version}\n" not in content:
        raise ValueError(f"Rendered {output.name} is missing the release version")
    output.write_text(content)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--digests", required=True, type=Path)
    parser.add_argument("--namespace", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--kustomize", default="kustomize")
    args = parser.parse_args()

    images = image_inventory(args.digests, args.namespace)
    args.output.mkdir(parents=True, exist_ok=True)
    kubernetes_dir = Path("deploy/kubernetes")
    render(kubernetes_dir, args.kustomize, images, args.version, args.output / "kubernetes-base.yaml")
    render(
        kubernetes_dir,
        args.kustomize,
        images,
        args.version,
        args.output / "kubernetes-hermes.yaml",
        hermes=True,
    )
    inventory = {"revision": args.revision, "images": images}
    (args.output / "images.json").write_text(json.dumps(inventory, indent=2) + "\n")


if __name__ == "__main__":
    main()
