"""Build deterministic, allowlisted companion archives without publishing them."""

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def archive(files: dict[str, bytes], windows: bool) -> bytes:
    """Use fixed archive timestamps and owners, preserving executable modes."""
    out = io.BytesIO()
    if windows:
        with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for name, data in sorted(files.items()):
                info = zipfile.ZipInfo(name, (2026, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = 0o644 << 16
                z.writestr(info, data)
    else:
        with gzip.GzipFile(fileobj=out, mode="wb", filename="", mtime=0) as gz:
            with tarfile.open(fileobj=gz, mode="w") as tar:
                for name, data in sorted(files.items()):
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.mode = 0o755 if name in {"steam-sage-companion", "install.sh"} else 0o644
                    info.mtime = 0
                    tar.addfile(info, io.BytesIO(data))
    return out.getvalue()


def main() -> None:
    """Build binaries and archives from a clean commit and print checksums."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "dist")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-alpha\.[0-9]+)?", args.version):
        raise SystemExit("Invalid version")
    if subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT).strip():
        raise SystemExit("Commit source changes before building release artifacts")
    source_ref = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    go_version = subprocess.check_output(["go", "version"], cwd=ROOT, text=True).strip()
    args.output.mkdir(parents=True, exist_ok=True)
    checksums = []
    docs = {"LICENSE": ROOT / "LICENSE", "README.md": ROOT / "README.md", "THIRD_PARTY_NOTICES.txt": ROOT / "THIRD_PARTY_NOTICES.txt"}
    docs.update({f"docs/{name}.md": ROOT / "docs" / f"{name}.md" for name in ["installation", "privacy", "protocol", "security"]})
    for target in ["linux", "windows"]:
        windows = target == "windows"
        binary_name = "steam-sage-companion.exe" if windows else "steam-sage-companion"
        binary = args.output / binary_name
        env = {**os.environ, "GOOS": target, "GOARCH": "amd64", "CGO_ENABLED": "0"}
        subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags", f"-s -w -X main.version={args.version}", "-o", str(binary), "./cmd/steam-sage-companion"], cwd=ROOT, env=env, check=True)
        files = {name: path.read_bytes() for name, path in docs.items()}
        files[binary_name] = binary.read_bytes()
        if not windows:
            files["install.sh"] = (ROOT / "packaging/linux/install.sh").read_bytes()
        files["release.json"] = (json.dumps({"version": args.version, "protocol": 1, "source_ref": source_ref, "go_version": go_version, "platform": f"{target}-amd64", "status": "experimental-unverified" if windows else "alpha"}, indent=2) + "\n").encode()
        name = f"steam-sage-companion-{args.version}-{target}-amd64.{ 'zip' if windows else 'tar.gz'}"
        data = archive(files, windows)
        (args.output / name).write_bytes(data)
        checksums.append(f"{hashlib.sha256(data).hexdigest()}  {name}")
    text = "\n".join(checksums) + "\n"
    (args.output / "SHA256SUMS").write_text(text)
    print(text, end="")


if __name__ == "__main__":
    main()
