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
