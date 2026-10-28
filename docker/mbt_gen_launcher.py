"""Runs the fizzbee MBT generator on the system python3 (docker image only).

The fizz wrapper's mbt-scaffold subcommand executes `python "$MBT_GEN_ZIP"`.
In the release tarball that is bazel's generator_bin.zip, whose bootstrap
extracts itself and re-executes the hermetic python toolchain embedded in
the zip. The docker image unpacks the zip's runfiles WITHOUT that toolchain
(see Dockerfile) and points MBT_GEN_ZIP at this file, which runs the same
generator sources and pip dependencies on the Debian python3.

The import path is assembled from the runfiles tree by shape rather than by
bazel repository name (see docker/parser-launcher).
"""
import glob
import os
import runpy
import sys

RUNFILES = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mbt_gen", "runfiles")

path = [os.path.join(RUNFILES, "_main")]
path += sorted(glob.glob(os.path.join(RUNFILES, "*", "site-packages")))
path += sorted(glob.glob(os.path.join(RUNFILES, "protobuf*", "python")))

# Replace sys.path[0] (this script's directory, /opt/fizzbee) so nothing
# there can shadow the generator's modules; keep the stdlib entries.
sys.path[:1] = path
sys.dont_write_bytecode = True

if __name__ == "__main__":
    runpy.run_module("mbt.generator.main", run_name="__main__", alter_sys=True)
