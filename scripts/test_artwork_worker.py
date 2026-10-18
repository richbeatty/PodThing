import ast
import pathlib
import subprocess
import sys
import unittest
import contextlib
import importlib.util
import io
import tempfile
from types import SimpleNamespace
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parent.parent
WORKER = ROOT / "firmware/direct/rootfs/usr/lib/podthing/artwork.py"

class ArtworkWorkerTests(unittest.TestCase):
    def test_worker_parses_without_a_connected_ipod(self):
        ast.parse(WORKER.read_text())
        result = subprocess.run(
            [sys.executable, str(WORKER)],
            capture_output=True, timeout=5, check=False
        )
        self.assertEqual(result.returncode, 2)

    def test_worker_never_uses_libgpod_write_or_sync_apis(self):
        text = WORKER.read_text()
        for forbidden in (
            "itdb_write", "itdb_start_sync", "itdb_stop_sync",
            "itdb_track_set_thumbnails", "itdb_cp_track_to_ipod",
        ):
            self.assertNotIn(forbidden, text)

    def test_worker_reports_artwork_db_missing_and_thumbnail_missing(self):
        spec = importlib.util.spec_from_file_location("podthing_artwork_worker", WORKER)
        worker = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(worker)
        fake_gpod = SimpleNamespace(
            itdb_parse=lambda root, err: 123,
            itdb_free=lambda db: None,
            itdb_track_by_id=lambda db, track: 456,
            itdb_track_get_thumbnail=lambda track, w, h: None,
        )
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            def run_once():
                output = io.StringIO()
                with mock.patch.object(worker, "load_library", return_value=(fake_gpod,None,None)), \
                     mock.patch.object(sys, "stdin", io.StringIO("100\\nQUIT\\n")), \
                     contextlib.redirect_stdout(output):
                    self.assertEqual(worker.run(str(root)),0)
                return output.getvalue()
            self.assertIn("NONE ARTWORK_DB_NOT_PRESENT", run_once())
            path = root / "iPod_Control" / "Artwork" / "ArtworkDB"
            path.parent.mkdir(parents=True)
            path.write_bytes(b"test")
            self.assertIn("NONE TRACK_HAS_NO_THUMBNAIL", run_once())

if __name__ == "__main__":
    unittest.main()
