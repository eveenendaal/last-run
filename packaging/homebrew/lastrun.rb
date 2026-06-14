# Reference formula for the FIRST submission of lastrun to homebrew-core.
#
# This file is not used by this repo's CI; it is the starting point for the
# initial manual pull request to https://github.com/Homebrew/homebrew-core.
# After the formula is accepted, the `.github/workflows/homebrew.yml` workflow
# keeps it up to date with `brew bump-formula-pr` on every release.
#
# To submit:
#   1. Pick the latest release tag (e.g. v1.0.63) and fill in `url` below.
#   2. Compute the source-tarball checksum:
#        curl -L https://github.com/eveenendaal/last-run/archive/refs/tags/vX.Y.Z.tar.gz | shasum -a 256
#   3. Copy this file (without these comments) into homebrew-core's Formula/l/lastrun.rb
#      and open a PR. Homebrew CI builds bottles for all supported platforms.
#
# SQLite is statically bundled via the rusqlite "bundled" feature, so the only
# dependency is the Rust toolchain at build time.
#
# NOTE before first submission: homebrew-core builds from source, so the binary
# reports the version in Cargo.toml. Today Cargo.toml says "0.2.0" while releases
# are tagged "v1.0.x" (the CI version bump is ephemeral and never committed).
# Align Cargo.toml's `version` with the release tag (and ideally commit the bump
# in the release flow) so `lastrun --version` matches the formula version; then
# the test below can assert the exact version.
class Lastrun < Formula
  desc "Track when commands were last executed and manage periodic tasks"
  homepage "https://github.com/eveenendaal/last-run"
  url "https://github.com/eveenendaal/last-run/archive/refs/tags/v1.0.63.tar.gz"
  sha256 "REPLACE_WITH_SOURCE_TARBALL_SHA256"
  license "MIT"

  depends_on "rust" => :build

  def install
    system "cargo", "install", *std_cargo_args
  end

  test do
    assert_match "lastrun", shell_output("#{bin}/lastrun --version")
  end
end
