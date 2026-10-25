"""Release boundary regressions; no installed helper is run."""
import hashlib
import io
from pathlib import Path
import tempfile
import json
import os
import shutil
import subprocess
import sys
import tarfile
import unittest
import zipfile

import build_release as release


class ReleaseTests(unittest.TestCase):
    def test_windows_payload_is_gui_and_has_provenance(self):
        args = release.go_build_args("0.1.0-alpha.3", "a" * 40, "windows", Path("helper.exe"))
        flags = args[args.index("-ldflags") + 1]
        self.assertIn("-H=windowsgui", flags)
        self.assertIn("internal/install.PackageVersion=0.1.0-alpha.3", flags)
        self.assertIn("internal/install.PackageSource=" + "a" * 40, flags)
        self.assertNotIn("packageqa", " ".join(args))

    def test_source_archive_excludes_untracked_and_private_artifacts(self):
        files = {"cmd/main.go": b"public", "go.mod": b"module public", "go.sum": b"", "LICENSE": b"MIT", "README.md": b"readme", "scripts/build_release.py": b"build", "docs/windows-preview.md": b"preview", ".github/workflows/ci.yml": b"ci", "settings.json": b"secret", ".env": b"secret", ".superpowers/report.md": b"private", "dist/helper.exe": b"binary", "internal/config/settings.json": b"private"}
        filtered = release.source_files(files)
        self.assertEqual(set(filtered), {"cmd/main.go", "go.mod", "go.sum", "LICENSE", "README.md", "scripts/build_release.py", "docs/windows-preview.md", ".github/workflows/ci.yml"})

    def test_source_archive_rejects_traversal_and_absolute_names(self):
        files = {"docs/../README.md": b"unsafe", "/docs/file.md": b"unsafe", "docs/good.md": b"good"}
        self.assertEqual(release.source_files(files), {"docs/good.md": b"good"})

    def test_public_output_rejects_qa_and_unlisted_entries(self):
        for name in ["steam-sage-companion-0.1.0-alpha.3-windows-amd64-QA-setup.exe", "settings.json", "windows-build.json", "steam-sage-companion.exe"]:
            with self.subTest(name=name), self.assertRaises(ValueError):
                release.validate_output({name: b"unwanted"}, "0.1.0-alpha.3", ["windows"])

    def test_windows_archive_repeats_exactly_and_contains_only_inputs(self):
        files = {"release.json": b'{"qa_adapter":false}', "steam-sage-companion.exe": b"MZ synthetic"}
        one, two = release.archive(files, True), release.archive(files, True)
        self.assertEqual(hashlib.sha256(one).digest(), hashlib.sha256(two).digest())
        with zipfile.ZipFile(io.BytesIO(one)) as z:
            self.assertEqual(set(z.namelist()), set(files))
            self.assertEqual(z.read("release.json"), files["release.json"])

    def test_source_zip_keeps_shell_entrypoints_executable(self):
        with zipfile.ZipFile(io.BytesIO(release.archive({"scripts/with-isolated-network.sh": b"#!/bin/sh\n"}, True))) as z:
            self.assertEqual((z.getinfo("scripts/with-isolated-network.sh").external_attr >> 16) & 0o777, 0o755)

    def test_ignored_embed_file_never_enters_public_payload(self):
        # Fetch only the explicit public HEAD at depth one, never other refs/history.
        # Build only: the companion executable is never run or connected to.
        with tempfile.TemporaryDirectory(prefix="sage-release-source-test-") as tmp:
            clone, output = Path(tmp) / "source", Path(tmp) / "candidate"
            clone.mkdir()
            source_ref = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=release.ROOT, text=True).strip()
            def git(*args):
                return subprocess.check_output(["git", *args], cwd=clone, stderr=subprocess.STDOUT, text=True).strip()
            git("init", "--quiet")
            git("fetch", "--quiet", "--depth=1", str(release.ROOT), source_ref)
            git("checkout", "--quiet", "--detach", "FETCH_HEAD")
            # Exercise the current builder under test, including an uncommitted fix,
            # as a committed synthetic fixture; no private branches are fetched.
            shutil.copyfile(Path(release.__file__), clone / "scripts/build_release.py")
            git("add", "scripts/build_release.py")
            git("-c", "user.name=Release Fixture", "-c", "user.email=release-fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "Synthetic builder under test")
            expected_ref, expected_tree = git("rev-parse", "HEAD"), git("rev-parse", "HEAD^{tree}")
            marker = b"SAGE_RELEASE_SYNTHETIC_IGNORED_SECRET_MUST_NOT_SHIP_2489"
            ignored = clone / "internal/bridge/web/settings.json"
            ignored.write_bytes(marker)
            self.assertEqual(git("status", "--porcelain"), "")
            subprocess.run([sys.executable, str(clone / "scripts/build_release.py"), "--version", "0.1.0-alpha.3", "--platform", "linux", "--output", str(output)], cwd=clone, env={**os.environ, "CGO_ENABLED": "0"}, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
            with tarfile.open(output / "steam-sage-companion-0.1.0-alpha.3-linux-amd64.tar.gz") as archive:
                binary = archive.extractfile("steam-sage-companion").read()
                metadata = json.load(archive.extractfile("release.json"))
            self.assertFalse(marker in binary, "Ignored web file leaked into compiled public helper")
            self.assertEqual(metadata["source_ref"], expected_ref)
            self.assertEqual(metadata["source_tree"], expected_tree)
            self.assertEqual(ignored.read_bytes(), marker, "Builder changed ambient ignored data")
            self.assertEqual(git("worktree", "list", "--porcelain").count("worktree "), 1, "Temporary build checkout was not removed")

    def test_output_must_be_empty_and_never_overwrites(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp)
            (out / "keep").write_text("keep")
            with self.assertRaises(ValueError):
                release.prepare_output(out)
            self.assertEqual((out / "keep").read_text(), "keep")


if __name__ == "__main__":
    unittest.main()
