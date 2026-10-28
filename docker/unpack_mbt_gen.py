"""Build-time helper (docker fetch stage): unpack mbt_gen.zip without its
bundled python toolchain. See Dockerfile and docker/mbt_gen_launcher.py.

usage: unpack_mbt_gen.py <mbt_gen.zip> <out_dir>
"""
import re
import sys
import zipfile

src, out = sys.argv[1], sys.argv[2]
# rules_python toolchain repos: rules_python++python+python_3_12_<triple>
# ('++'/'+' era) or rules_python~~python~python_3_12_<triple> ('~~'/'~' era).
toolchain = re.compile(r"^runfiles/([^/]*python[+~]python_3_[^/]*)/")
with zipfile.ZipFile(src) as z:
    names = z.namelist()
    toolchain_roots = sorted({m.group(1) for n in names if (m := toolchain.match(n))})
    if len(toolchain_roots) != 1:
        sys.exit("ERROR: expected exactly 1 bundled python toolchain in %s, found %s (zip layout changed?)"
                 % (src, toolchain_roots or "none"))
    toolchain_entries = [n for n in names if toolchain.match(n)]
    # Keep the runfiles tree minus the toolchain. Entries outside runfiles/
    # (__main__.py, __init__.py) are bazel's self-extracting bootstrap, which
    # hardcodes the toolchain interpreter and is replaced by the launcher.
    keep = [n for n in names if n.startswith("runfiles/") and not toolchain.match(n)]
    bootstrap = [n for n in names if not n.startswith("runfiles/")]
    assert len(keep) + len(toolchain_entries) + len(bootstrap) == len(names)
    assert not any(toolchain.match(n) for n in keep)
    z.extractall(out, keep)
print("%s: kept %d runfiles entries; dropped toolchain %s (%d entries) and bazel bootstrap %s"
      % (src, len(keep), toolchain_roots[0], len(toolchain_entries), bootstrap))
