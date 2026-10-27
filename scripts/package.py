"""Package the native SDK v2 reference plugin, with no outer ZIP directory."""
import json
import pathlib
import subprocess
import zipfile

root = pathlib.Path(__file__).resolve().parent.parent
manifest = json.loads((root / "plugin.json").read_text())
platform = subprocess.check_output(["go", "env", "GOOS", "GOARCH"], text=True).split()
output = root / "dist" / f"example-{manifest['version']}-{'-'.join(platform)}.zip"
with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED) as archive:
    archive.write(root / "dist" / "example", "example")
    archive.write(root / "plugin.json", "plugin.json")
    archive.write(root / "config.example.yaml", "config.yaml")
print(output)
