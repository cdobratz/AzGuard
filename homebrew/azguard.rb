class Azguard < Formula
  desc "One command to make sure your Azure free tier doesn't surprise you with a bill"
  homepage "https://github.com/cdobratz/AzGuard"
  license "MIT"
  version "1.0.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256"
    end
  end

  def install
    bin.install "azguard"
  end

  test do
    system "#{bin}/azguard", "--version"
  end
end
