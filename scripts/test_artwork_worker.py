import ast
import pathlib
import subprocess
import sys
import unittest

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

if __name__ == "__main__":
    unittest.main()
