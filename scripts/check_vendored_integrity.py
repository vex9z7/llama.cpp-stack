#!/usr/bin/env python3
"""Verify vendored upstream snapshots have not been edited in-tree.

Each vendored snapshot directory has a SHA256SUMS manifest generated from the
exact files committed into the repo. CI runs this check so accidental edits,
deletions, additions, or regeneration drift in vendored upstream snapshots fail
fast. To intentionally bump a snapshot, run the corresponding update flow and
regenerate the manifest in the same review.
"""
from __future__ import annotations

import argparse
import hashlib
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SNAPSHOTS = [ROOT / "openai-openapi", ROOT / "llamacpp-upstream"]
MANIFEST = "SHA256SUMS"


def rel(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def snapshot_files(root: Path) -> list[Path]:
    files: list[Path] = []
    for path in root.rglob("*"):
        if not path.is_file():
            continue
        if path.name == MANIFEST:
            continue
        if any(part in {".git", "__pycache__"} for part in path.parts):
            continue
        files.append(path)
    return sorted(files, key=lambda p: rel(p))


def write_manifest(root: Path) -> None:
    lines = [f"{sha256(path)}  {rel(path)}\n" for path in snapshot_files(root)]
    (root / MANIFEST).write_text("".join(lines), encoding="utf-8")


def read_manifest(path: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for lineno, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip() or line.startswith("#"):
            continue
        try:
            digest, file_path = line.split(None, 1)
        except ValueError as exc:
            raise AssertionError(f"{rel(path)}:{lineno}: invalid manifest line") from exc
        file_path = file_path.strip()
        out[file_path] = digest
    return out


def check_manifest(root: Path) -> None:
    manifest_path = root / MANIFEST
    if not manifest_path.exists():
        raise AssertionError(f"missing vendored integrity manifest: {rel(manifest_path)}")
    expected = read_manifest(manifest_path)
    actual_files = {rel(path): path for path in snapshot_files(root)}
    missing = sorted(set(expected) - set(actual_files))
    extra = sorted(set(actual_files) - set(expected))
    if missing:
        raise AssertionError(f"{rel(root)} manifest references missing files: {missing}")
    if extra:
        raise AssertionError(f"{rel(root)} contains unmanifested files: {extra}")
    mismatches = []
    for file_path, want in sorted(expected.items()):
        got = sha256(ROOT / file_path)
        if got != want:
            mismatches.append(f"{file_path}: sha256 {got}, want {want}")
    if mismatches:
        raise AssertionError("vendored snapshot content drift:\n" + "\n".join(mismatches))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true", help="rewrite SHA256SUMS manifests from current vendored files")
    args = parser.parse_args()

    for root in SNAPSHOTS:
        if args.write:
            write_manifest(root)
        else:
            check_manifest(root)
    if args.write:
        print("vendored integrity manifests updated.")
    else:
        print("vendored integrity manifests ok.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        raise
