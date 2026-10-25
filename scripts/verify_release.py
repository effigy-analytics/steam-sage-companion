"""Inspect clean Windows preview deliverables and compare independent builds."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import struct
import zipfile

from build_release import source_files, validate_output


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def gui_executable(data: bytes) -> bool:
    """Check actual PE subsystem, not a filename or build-log assertion."""
    if len(data) < 64 or data[:2] != b"MZ":
        return False
    pe = struct.unpack_from("<I", data, 0x3C)[0]
    return (pe + 94 <= len(data) and data[pe:pe + 4] == b"PE\0\0"
            and struct.unpack_from("<H", data, pe + 24 + 68)[0] == 2)


def inspect(root: Path, version: str, source_ref: str) -> dict[str, bytes]:
    """Reject extra files, bad checksums, non-GUI/QA payloads and leaked sources."""
    if any(not p.is_file() for p in root.iterdir()):
        raise ValueError("Release directory contains a non-file")
    files = {p.name: p.read_bytes() for p in root.iterdir() if p.name != "SHA256SUMS"}
    validate_output(files, version, ["windows"])
    expected = "".join(f"{digest(data)}  {name}\n" for name, data in sorted(files.items()))
    if (root / "SHA256SUMS").read_text(encoding="utf-8") != expected:
        raise ValueError("Checksums do not exactly cover deliverables")
    prefix = f"steam-sage-companion-{version}"
    setup = files[prefix + "-windows-amd64-setup.exe"]
    if not gui_executable(setup) or b'requestedExecutionLevel level="asInvoker"' not in setup:
        raise ValueError("Setup must be a per-user GUI executable")
    if b"SteamSageCompanionQA" in setup or "SteamSageCompanionQA".encode("utf-16le") in setup:
        raise ValueError("Setup contains QA namespace")
    with zipfile.ZipFile(io.BytesIO(files[prefix + "-windows-amd64.zip"])) as z:
        names = z.namelist()
        expected_names = {"LICENSE", "README.md", "THIRD_PARTY_NOTICES.txt", "steam-sage-companion.exe", "release.json"}
        expected_names.update("docs/" + n + ".md" for n in ["installation", "privacy", "protocol", "security", "windows-preview"])
        if len(names) != len(set(names)) or set(names) != expected_names:
            raise ValueError("Portable ZIP contains unexpected or missing payloads")
        if not gui_executable(z.read("steam-sage-companion.exe")):
            raise ValueError("Portable helper is not a GUI executable")
        metadata = json.loads(z.read("release.json"))
        if (metadata["version"] != version or metadata["source_ref"] != source_ref
                or metadata["status"] != "preview" or metadata["qa_adapter"] is not False
                or metadata["platform"] != "windows-amd64" or metadata["protocol"] != 1):
            raise ValueError("Portable provenance/status does not match reviewed public source")
    with zipfile.ZipFile(io.BytesIO(files[prefix + "-source.zip"])) as z:
        names = z.namelist()
        inputs = {name: z.read(name) for name in names}
        if len(names) != len(inputs) or source_files(inputs) != inputs:
            raise ValueError("Source ZIP contains unallowlisted or private content")
        required = {"go.mod", "go.sum", "LICENSE", "scripts/build_release.py", "scripts/build_windows_installer.ps1", "packaging/windows/installer.nsi", "cmd/steam-sage-companion/main.go"}
        if not required.issubset(inputs):
            raise ValueError("Source ZIP lacks required build sources")
    return files


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("first", type=Path)
    parser.add_argument("second", type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--source-ref", required=True)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    one, two = (inspect(root, args.version, args.source_ref) for root in [args.first, args.second])
    report = {"source_ref": args.source_ref, "version": args.version, "files": {}}
    for name, data in one.items():
        other = two[name]
        detail = {"first_sha256": digest(data), "second_sha256": digest(other), "identical": data == other}
        if data != other:
            detail.update({"first_bytes": len(data), "second_bytes": len(other), "first_different_offset": next((i for i, (a, b) in enumerate(zip(data, other)) if a != b), min(len(data), len(other)))})
        report["files"][name] = detail
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    if one != two:
        raise SystemExit("Unsigned builds differ; inspect repeatability report before release")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
