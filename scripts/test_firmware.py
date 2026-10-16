"""Tests use a synthetic format fixture, never a flashable operating system."""
import importlib.util
import io
import pathlib
import struct
import tempfile
import unittest
import zipfile
import zlib

spec = importlib.util.spec_from_file_location("firmware_validator", pathlib.Path(__file__).with_name("validate-firmware.py"))
validator = importlib.util.module_from_spec(spec)
spec.loader.exec_module(validator)


def fixture_disk():
    disk = bytearray(1024 * 1024)
    disk[510:512] = b"\x55\xaa"
    disk[450] = 0xEE
    table = bytearray(128 * 128)
    for i, name in enumerate(("env", "boot_a", "root_a", "boot_b", "root_b", "bandaid")):
        start = 40 + i * 32
        table[i * 128 : i * 128 + 16] = bytes([i + 1]) * 16
        struct.pack_into("<QQ", table, i * 128 + 32, start, start + 31)
        encoded = name.encode("utf-16-le")
        table[i * 128 + 56 : i * 128 + 56 + len(encoded)] = encoded
        if name.startswith("root_"):
            offset = start * 512 + 1080
            disk[offset : offset + 2] = b"\x53\xef"
    disk[1024 : 1024 + len(table)] = table
    header = bytearray(512)
    header[:8] = b"EFI PART"
    struct.pack_into("<I", header, 12, 92)
    struct.pack_into("<QIII", header, 72, 2, 128, 128, zlib.crc32(table))
    struct.pack_into("<I", header, 16, zlib.crc32(header[:92]))
    disk[512:1024] = header
    return disk


class FirmwareTests(unittest.TestCase):
    def test_gpt_and_ext4_contract(self):
        disk = fixture_disk()
        parts = validator.check_gpt(io.BytesIO(disk), len(disk))
        self.assertEqual(len(parts), 6)
        disk[530] ^= 1
        with self.assertRaisesRegex(ValueError, "CRC"):
            validator.check_gpt(io.BytesIO(disk), len(disk))

    def test_application_archive_cannot_be_flushed_as_firmware(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "app.zip"
            with zipfile.ZipFile(path, "w") as archive:
                archive.writestr("podthing", "application only")
            with self.assertRaisesRegex(ValueError, "missing boot artifacts"):
                validator.validate(path)

    def test_wrong_rootfs_magic_is_rejected(self):
        disk = fixture_disk()
        disk[(40 + 2 * 32) * 512 + 1080] = 0
        with self.assertRaisesRegex(ValueError, "ext4"):
            validator.check_gpt(io.BytesIO(disk), len(disk))


if __name__ == "__main__":
    unittest.main()
