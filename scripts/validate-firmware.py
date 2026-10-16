#!/usr/bin/env python3
"""Validate the flash contract and GPT/rootfs signatures, never flash a device.

These checks do not establish that an image boots or that iPod hardware works.
"""
import json
import struct
import sys
import zipfile
import zlib
from pathlib import PurePosixPath


def require(condition, message):
    if not condition:
        raise ValueError(message)


def read_at(stream, offset, count):
    stream.seek(offset)
    data = stream.read(count)
    require(len(data) == count, "truncated disk image")
    return data


def check_gpt(stream, image_size):
    mbr = read_at(stream, 0, 512)
    require(mbr[510:512] == b"\x55\xaa", "missing protective MBR signature")
    require(mbr[450] == 0xEE, "MBR is not GPT protective")
    header = bytearray(read_at(stream, 512, 512))
    require(header[:8] == b"EFI PART", "missing GPT header")
    size, expected = struct.unpack_from("<II", header, 12)
    require(92 <= size <= 512, "invalid GPT header size")
    struct.pack_into("<I", header, 16, 0)
    require(zlib.crc32(header[:size]) == expected, "GPT header CRC mismatch")
    lba, entries, entry_size, expected_entries = struct.unpack_from("<QIII", header, 72)
    require(1 <= entries <= 4096 and 128 <= entry_size <= 1024, "invalid GPT entries")
    raw = read_at(stream, lba * 512, entries * entry_size)
    require(zlib.crc32(raw) == expected_entries, "GPT entries CRC mismatch")
    partitions = {}
    intervals = []
    for i in range(entries):
        record = raw[i * entry_size : (i + 1) * entry_size]
        if record[:16] == bytes(16):
            continue
        start, end = struct.unpack_from("<QQ", record, 32)
        name = record[56:128].decode("utf-16-le").split("\0", 1)[0]
        require(name and name not in partitions, "duplicate/unnamed GPT partition")
        require(start >= 34 and end >= start, "invalid GPT partition bounds")
        require((end + 1) * 512 <= image_size, "partition extends past image")
        partitions[name] = {"start": start, "end": end}
        intervals.append((start, end))
    intervals.sort()
    require(all(a[1] < b[0] for a, b in zip(intervals, intervals[1:])), "overlapping GPT partitions")
    require(set(("env", "boot_a", "root_a", "boot_b", "root_b", "bandaid")) <= partitions.keys(), "required A/B partitions missing")
    for name in ("root_a", "root_b"):
        magic = read_at(stream, partitions[name]["start"] * 512 + 1024 + 56, 2)
        require(magic == b"\x53\xef", f"{name} is not the expected ext4 rootfs")
    return partitions


def validate(path):
    with zipfile.ZipFile(path) as archive:
        names = archive.namelist()
        require(len(names) == len(set(names)), "duplicate archive entries")
        for name in names:
            p = PurePosixPath(name)
            require(not p.is_absolute() and ".." not in p.parts, "unsafe ZIP entry path")
        require({"meta.json", "superbird-boot.bin", "superbird.wic"} <= set(names), "missing boot artifacts")
        meta = json.loads(archive.read("meta.json"))
        require(meta.get("metadataVersion") == 2, "expected metadataVersion 2")
        require(meta.get("name") == "podthing-image", "unexpected firmware identity")
        steps = meta.get("steps", [])
        require(len(steps) == 4, "unexpected flashing steps")
        require(steps[0] == {"type": "bulkcmd", "value": "amlmmc key"}, "unexpected initialization command")
        for step, hwpart in zip(steps[1:3], (1, 2)):
            require(step == {"type": "writeBootPartition", "value": {"hwpart": hwpart, "data": {"filePath": "superbird-boot.bin"}}}, "unexpected boot-partition write")
        require(steps[3] == {"type": "writeUserArea", "value": {"lba": 0, "data": {"filePath": "superbird.wic"}, "sparse": True}}, "unexpected user-area write")
        require(1024 < archive.getinfo("superbird-boot.bin").file_size < 16 * 1024 * 1024, "invalid signed boot artifact size")
        require(archive.testzip() is None, "archive CRC failure")
        with archive.open("superbird.wic") as stream:
            partitions = check_gpt(stream, archive.getinfo("superbird.wic").file_size)
    return {"name": meta["name"], "metadata_version": 2, "partitions": partitions, "format_checks": "passed", "hardware_boot": "not tested"}


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("Usage: python3 scripts/validate-firmware.py IMAGE.zip")
    try:
        print(json.dumps(validate(sys.argv[1]), indent=2))
    except (ValueError, OSError, zipfile.BadZipFile, KeyError) as error:
        raise SystemExit(f"Firmware validation failed: {error}") from error
