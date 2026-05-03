#!/usr/bin/env python3
# pyright: reportAny=false, reportExplicitAny=false, reportUnknownMemberType=false, reportUnknownVariableType=false, reportUnknownArgumentType=false, reportUnusedCallResult=false
"""Fetch PostgreSQL client tools from conda-forge and stage them for go:embed."""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import re
import shutil
import tarfile
import tempfile
import urllib.request
import zipfile
from dataclasses import dataclass
from pathlib import Path

import zstandard

CHANNEL = "https://conda.anaconda.org/conda-forge"
PLATFORMS = {
    "windows-amd64": "win-64",
    "linux-amd64": "linux-64",
    "darwin-amd64": "osx-64",
    "darwin-arm64": "osx-arm64",
}


@dataclass(frozen=True)
class Package:
    filename: str
    name: str
    version: str
    build: str
    sha256: str
    depends: tuple[str, ...]
    subdir: str

    @property
    def url(self) -> str:
        return f"{CHANNEL}/{self.subdir}/{self.filename}"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", choices=sorted(PLATFORMS), help="platform to fetch")
    parser.add_argument("--all", action="store_true", help="fetch all supported platforms")
    parser.add_argument("--version", default="18.3", help="PostgreSQL version to fetch")
    parser.add_argument("--out", default="embed/bin", help="staging directory")
    parser.add_argument("--embed-out", default="internal/engine/pgtools/bin", help="go:embed staging directory")
    args = parser.parse_args()
    if not args.all and not args.platform:
        parser.error("pass --platform or --all")
    platforms = sorted(PLATFORMS) if args.all else [args.platform]
    for platform in platforms:
        fetch_platform(platform, args.version, Path(args.out), Path(args.embed_out))
    return 0


def fetch_platform(platform: str, version: str, out: Path, embed_out: Path) -> None:
    subdir = PLATFORMS[platform]
    print(f"fetch-pgtools-conda: resolving {platform} from conda-forge/{subdir}")
    packages = load_packages(subdir)
    selected = resolve(packages, version)
    print(f"fetch-pgtools-conda: selected {len(selected)} packages for {platform}")
    with tempfile.TemporaryDirectory(prefix="pgsync-pgtools-") as tmp:
        extract_root = Path(tmp) / "extract"
        extract_root.mkdir(parents=True)
        for package in selected:
            archive = download(package, Path(tmp))
            extract_package(archive, extract_root / package.name)
        stage_dir = out / platform
        embed_dir = embed_out / platform
        stage_payload(platform, extract_root, stage_dir)
        if embed_dir.exists():
            shutil.rmtree(embed_dir)
        shutil.copytree(stage_dir, embed_dir)
        write_sha256s(stage_dir)
        write_sha256s(embed_dir)
    print(f"fetch-pgtools-conda: staged {platform} in {out / platform} and {embed_out / platform}")


def load_packages(subdir: str) -> dict[str, list[Package]]:
    url = f"{CHANNEL}/{subdir}/repodata.json"
    with urllib.request.urlopen(url, timeout=120) as resp:
        data = json.load(resp)
    records: dict[str, list[Package]] = {}
    for section in ("packages.conda", "packages"):
        for filename, rec in data.get(section, {}).items():
            package = Package(
                filename=filename,
                name=rec["name"],
                version=rec["version"],
                build=rec.get("build", ""),
                sha256=rec.get("sha256", ""),
                depends=tuple(rec.get("depends", ())),
                subdir=subdir,
            )
            records.setdefault(package.name, []).append(package)
    for values in records.values():
        values.sort(key=package_sort_key, reverse=True)
    return records


def resolve(packages: dict[str, list[Package]], pg_version: str) -> list[Package]:
    root = choose(packages, "postgresql", pg_version)
    selected: dict[str, Package] = {}
    queue = [root]
    while queue:
        package = queue.pop(0)
        current = selected.get(package.name)
        if current is not None:
            continue
        selected[package.name] = package
        for dep in package.depends:
            name, constraint = parse_dep(dep)
            if not name or name.startswith("__"):
                continue
            if name in selected:
                continue
            if name not in packages:
                continue
            queue.append(choose(packages, name, constraint))
    return sorted(selected.values(), key=lambda p: p.name)


def parse_dep(dep: str) -> tuple[str, str]:
    parts = dep.split()
    if not parts:
        return "", ""
    name = parts[0]
    return name, " ".join(parts[1:])


def choose(packages: dict[str, list[Package]], name: str, constraint: str) -> Package:
    candidates = packages.get(name, [])
    for package in candidates:
        if satisfies(package, constraint):
            return package
    raise RuntimeError(f"no conda-forge package satisfies {name} {constraint!r}")


def satisfies(package: Package, constraint: str) -> bool:
    constraint = constraint.strip()
    if not constraint:
        return True
    parts = constraint.split()
    if parts and re.fullmatch(r"\d+(?:\.\d+)*(?:[A-Za-z0-9_.-]*)?", parts[0]):
        if package.version != parts[0]:
            return False
        if len(parts) > 1 and package.build != parts[1]:
            return False
        return True
    for part in constraint.split(","):
        part = part.strip()
        if not part:
            continue
        match = re.match(r"(>=|<=|==|=|>|<)\s*([^, ]+)", part)
        if not match:
            continue
        op, want = match.groups()
        if not compare_version(package.version, op, want):
            return False
    return True


def compare_version(got: str, op: str, want: str) -> bool:
    left = parse_version(got)
    right = parse_version(want)
    if op in ("=", "=="):
        return left == right
    if op == ">=":
        return left >= right
    if op == "<=":
        return left <= right
    if op == ">":
        return left > right
    if op == "<":
        return left < right
    return True


def parse_version(raw: str):
	parts = tuple(int(part) for part in re.findall(r"\d+", raw))
	return parts if parts else (0,)


def package_sort_key(package: Package):
    return (parse_version(package.version), package.build, package.filename.endswith(".conda"))


def download(package: Package, tmp: Path) -> Path:
    dest = tmp / package.filename
    if dest.exists():
        return dest
    print(f"fetch-pgtools-conda: downloading {package.filename}")
    with urllib.request.urlopen(package.url, timeout=180) as resp:
        data = resp.read()
    digest = hashlib.sha256(data).hexdigest()
    if package.sha256 and digest != package.sha256:
        raise RuntimeError(f"checksum mismatch for {package.filename}: got {digest} want {package.sha256}")
    dest.write_bytes(data)
    return dest


def extract_package(archive: Path, dest: Path) -> None:
    dest.mkdir(parents=True, exist_ok=True)
    if archive.suffix == ".conda":
        with zipfile.ZipFile(archive) as zf:
            pkg_name = next(name for name in zf.namelist() if name.startswith("pkg-") and name.endswith(".tar.zst"))
            compressed = zf.read(pkg_name)
        data = zstandard.ZstdDecompressor().decompress(compressed)
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:") as tf:
            safe_extract(tf, dest)
        return
    if archive.name.endswith(".tar.bz2"):
        with tarfile.open(archive, mode="r:bz2") as tf:
            safe_extract(tf, dest)
        return
    raise RuntimeError(f"unsupported conda package archive: {archive}")


def safe_extract(tf: tarfile.TarFile, dest: Path) -> None:
    for member in tf.getmembers():
        target = dest / member.name
        if not is_safe_path(dest, target):
            raise RuntimeError(f"unsafe path in package: {member.name}")
    tf.extractall(dest)


def is_safe_path(root: Path, target: Path) -> bool:
    root = root.resolve()
    target = target.resolve(strict=False)
    return root == target or root in target.parents


def stage_payload(platform: str, extract_root: Path, stage_dir: Path) -> None:
    if stage_dir.exists():
        shutil.rmtree(stage_dir)
    stage_dir.mkdir(parents=True)
    (stage_dir / ".gitkeep").write_text("")
    for path in extract_root.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(extract_root)
        rel_parts = rel.parts[1:]
        if not rel_parts:
            continue
        package_rel = Path(*rel_parts).as_posix()
        if should_stage(platform, package_rel):
            copy_file(path, stage_dir / package_rel)
    verify_staged_binaries(platform, stage_dir)


def should_stage(platform: str, rel: str) -> bool:
    base = Path(rel).name
    if platform.startswith("windows-"):
        return rel.startswith("Library/bin/") and (base.lower().endswith(".dll") or base in {"pg_dump.exe", "pg_restore.exe"})
    if rel in {"bin/pg_dump", "bin/pg_restore"}:
        return True
    if rel.startswith("lib/"):
        return ".so" in base or base.endswith(".dylib") or ".dylib." in base
    return False


def copy_file(src: Path, dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if src.is_symlink():
        src = src.resolve()
    shutil.copy2(src, dest)
    try:
        if dest.name in {"pg_dump", "pg_restore"} or ".so" in dest.name or dest.name.endswith(".dylib"):
            dest.chmod(0o755)
    except OSError:
        pass


def verify_staged_binaries(platform: str, stage_dir: Path) -> None:
    if platform.startswith("windows-"):
        names = ["pg_dump.exe", "pg_restore.exe"]
    else:
        names = ["pg_dump", "pg_restore"]
    for name in names:
        if not any(path.name == name for path in stage_dir.rglob("*")):
            raise RuntimeError(f"staged payload for {platform} is missing {name}")


def write_sha256s(stage_dir: Path) -> None:
    rows = []
    for path in sorted(stage_dir.rglob("*")):
        if not path.is_file() or path.name in {".gitkeep", "SHA256SUMS"}:
            continue
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        rows.append(f"{digest}  {path.relative_to(stage_dir).as_posix()}")
    (stage_dir / "SHA256SUMS").write_text("\n".join(rows) + "\n")


if __name__ == "__main__":
    raise SystemExit(main())
