class Azguard < Formula
  desc "One command to make sure your Azure free tier doesn't surprise you with a bill"
  homepage "https://github.com/cdobratz/AzGuard"
  license "MIT"
  version "1.0.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_darwin_arm64.zip"
      sha256 "a731db0dfa64b54401c76c94581062dd09b2c0baa4aeab6c31ed1aebf6a430d4"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_darwin_amd64.zip"
      sha256 "e95d38af8152f788e91bc72100259e0a27c652c3ce887b2eaae0272cac3e6536"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_linux_arm64.tar.gz"
      sha256 "5d85f466db6cce5241a65fccbb3c9ce825452a550392b7e98c5285a77b0fe1f7"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_linux_amd64.tar.gz"
      sha256 "38e562ca7cfad603b83aec9ce8508393e43b0d7dbff515189624a10c8f9d3a94"
    end
  end

  def install
    bin.install "azguard"
  end

  test do
    system "#{bin}/azguard", "--version"
  end
end
