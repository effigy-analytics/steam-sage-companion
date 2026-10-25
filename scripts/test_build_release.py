"""Release boundary regressions; no installed helper is run."""
import hashlib
import io
from pathlib import Path
import tempfile
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

    def test_output_must_be_empty_and_never_overwrites(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp)
            (out / "keep").write_text("keep")
            with self.assertRaises(ValueError):
                release.prepare_output(out)
            self.assertEqual((out / "keep").read_text(), "keep")


if __name__ == "__main__":
    unittest.main()
