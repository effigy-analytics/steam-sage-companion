"""Build deterministic, allowlisted companion archives without publishing them."""

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import tarfile
import tempfile
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
                info.create_system = 3
                info.external_attr = (0o755 if name.endswith(".sh") else 0o644) << 16
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


def go_build_args(version: str, source_ref: str, target: str, binary: Path) -> list[str]:
    """Compile a provenance-stamped public payload; Windows has no console."""
    flags = f"-s -w -X main.version={version}"
    if target == "windows":
        flags += (f" -H=windowsgui -X github.com/effigy-analytics/steam-sage-companion/internal/install.PackageVersion={version}"
                  f" -X github.com/effigy-analytics/steam-sage-companion/internal/install.PackageSource={source_ref}")
    return ["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags", flags,
            "-o", str(binary), "./cmd/steam-sage-companion"]


def source_files(files: dict[str, bytes]) -> dict[str, bytes]:
    """Only public build, test, documentation and protocol sources enter source ZIP."""
    roots = {"cmd", "internal", "scripts", "packaging", "protocol", "docs", ".github"}
    top = {"go.mod", "go.sum", "LICENSE", "README.md", "THIRD_PARTY_NOTICES.txt", ".gitignore", ".gitattributes"}
    suffixes = {".go", ".py", ".ps1", ".cs", ".sh", ".cjs", ".mjs", ".json", ".md", ".txt", ".yml", ".yaml", ".nsi", ".nsh", ".html", ".js", ".css"}
    result = {}
    for name, data in files.items():
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts or "\\" in name or ":" in name:
            continue
        if path.name in {"settings.json", "install.json", "package.ini"}:
            continue
        if name in top or (path.parts[0] in roots and path.suffix in suffixes):
            if re.search(rb"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|(?:ghp_|github_pat_)[A-Za-z0-9_]{30,}|AKIA[A-Z0-9]{16}", data):
                raise ValueError(f"Credential-like content in source: {name}")
            result[name] = data
    return result


def prepare_output(output: Path) -> None:
    """Do not mix candidates with earlier build or private work files."""
    if output.exists() and any(output.iterdir()):
        raise ValueError("Release output must be an empty directory")
    output.mkdir(parents=True, exist_ok=True)


def validate_output(files: dict[str, bytes], version: str, targets: list[str]) -> None:
    """The publishable directory contains only named release deliverables."""
    prefix = f"steam-sage-companion-{version}"
    expected = {f"{prefix}-source.zip"}
    if "linux" in targets:
        expected.add(f"{prefix}-linux-amd64.tar.gz")
    if "windows" in targets:
        expected.update({f"{prefix}-windows-amd64.zip", f"{prefix}-windows-amd64-setup.exe"})
    if set(files) != expected:
        raise ValueError("Unexpected or missing release deliverables: " + str(set(files) ^ expected))


def main() -> None:
    """Build production artifacts in private scratch space, then export allowlist."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--platform", choices=["all", "linux", "windows"], default="all")
    parser.add_argument("--output", type=Path, default=ROOT / "dist")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-alpha\.[0-9]+)?", args.version):
        raise SystemExit("Invalid version")
    if subprocess.check_output(["git", "status", "--porcelain"], cwd=ROOT).strip():
        raise SystemExit("Commit source changes before building release artifacts")
    targets = ["linux", "windows"] if args.platform == "all" else [args.platform]
    if "windows" in targets and os.name != "nt":
        raise SystemExit("Build Windows Setup on native Windows with the pinned NSIS toolchain")
    source_ref = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    source_tree = subprocess.check_output(["git", "rev-parse", "HEAD^{tree}"], cwd=ROOT, text=True).strip()
    go_version = subprocess.check_output(["go", "version"], cwd=ROOT, text=True).strip()
    tracked = subprocess.check_output(["git", "archive", "--format=tar", "HEAD"], cwd=ROOT)
    with tarfile.open(fileobj=io.BytesIO(tracked)) as source:
        inputs = {m.name: source.extractfile(m).read() for m in source.getmembers() if m.isfile()}
    sources = source_files(inputs)
    files = {f"steam-sage-companion-{args.version}-source.zip": archive(sources, True)}
    docs = {name: inputs[name] for name in ["LICENSE", "README.md", "THIRD_PARTY_NOTICES.txt"]}
    docs.update({f"docs/{name}.md": inputs[f"docs/{name}.md"] for name in ["installation", "privacy", "protocol", "security", "windows-preview"]})
    prepare_output(args.output)
    with tempfile.TemporaryDirectory(prefix="sage-public-build-") as scratch:
        for target in targets:
            windows = target == "windows"
            binary_name = "steam-sage-companion.exe" if windows else "steam-sage-companion"
            work = Path(scratch) / target
            work.mkdir()
            metadata = {"version": args.version, "protocol": 1, "source_ref": source_ref, "source_tree": source_tree,
                        "go_version": go_version, "platform": f"{target}-amd64", "status": "preview" if windows else "alpha", "qa_adapter": False}
            if windows:
                subprocess.run(["pwsh", "-NoProfile", "-File", str(ROOT / "scripts/build_windows_installer.ps1"),
                                "-Version", args.version, "-Output", str(work)], cwd=ROOT, check=True)
                build = json.loads((work / "windows-build.json").read_text(encoding="utf-8-sig"))
                if build["qa_adapter"] or build["source_ref"] != source_ref:
                    raise ValueError("Installer provenance mismatch or QA adapter")
                metadata.update({"nsis": build["nsis"], "nsis_sha256": build["nsis_sha256"]})
                setup = f"steam-sage-companion-{args.version}-windows-amd64-setup.exe"
                files[setup] = (work / setup).read_bytes()
            else:
                env = {**os.environ, "GOOS": target, "GOARCH": "amd64", "CGO_ENABLED": "0"}
                subprocess.run(go_build_args(args.version, source_ref, target, work / binary_name), cwd=ROOT, env=env, check=True)
            payload = {**docs, binary_name: (work / binary_name).read_bytes()}
            if not windows:
                payload["install.sh"] = inputs["packaging/linux/install.sh"]
            payload["release.json"] = (json.dumps(metadata, indent=2) + "\n").encode()
            extension = "zip" if windows else "tar.gz"
            files[f"steam-sage-companion-{args.version}-{target}-amd64.{extension}"] = archive(payload, windows)
    validate_output(files, args.version, targets)
    checksums = "".join(f"{hashlib.sha256(data).hexdigest()}  {name}\n" for name, data in sorted(files.items()))
    for name, data in files.items():
        (args.output / name).write_bytes(data)
    (args.output / "SHA256SUMS").write_text(checksums, encoding="utf-8", newline="\n")
    print(checksums, end="")


if __name__ == "__main__":
    main()
