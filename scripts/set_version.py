"""Set the example plugin version before building its embedded manifest."""
import json
import pathlib
import re
import sys


root = pathlib.Path(__file__).resolve().parent.parent
version = sys.argv[1] if len(sys.argv) == 2 else ""
if not re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?", version):
    raise SystemExit("VERSION 必须是语义版本号，例如 1.0.2")

manifest_path = root / "plugin.json"
catalog_path = root / "catalog.example.json"
registration_path = root / "registration.example.json"
manifest = json.loads(manifest_path.read_text())
catalog = json.loads(catalog_path.read_text())
registration = json.loads(registration_path.read_text())
old_version = manifest["version"]
if catalog["manifest"]["version"] != old_version or registration["modules"][0]["manifest"]["version"] != old_version:
    raise SystemExit("插件清单、目录和注册示例的当前版本不一致")

manifest["version"] = version
catalog["manifest"]["version"] = version
registration["modules"][0]["manifest"]["version"] = version
for platform, path in catalog["artifacts"].items():
    expected = f"/{old_version}/example-{old_version}-{platform}.zip"
    if not path.endswith(expected):
        raise SystemExit(f"目录中的 {platform} 插件包地址与当前版本不一致")
    catalog["artifacts"][platform] = path[: -len(expected)] + f"/{version}/example-{version}-{platform}.zip"

container = catalog.get("container")
if container and container.get("image", "").endswith(f":{old_version}"):
    container["image"] = container["image"][: -len(old_version)] + version

for path, data in ((manifest_path, manifest), (catalog_path, catalog), (registration_path, registration)):
    if version != old_version:
        path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
