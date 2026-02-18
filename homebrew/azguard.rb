class Azguard < Formula
  desc "One command to make sure your Azure free tier doesn't surprise you with a bill"
  homepage "https://github.com/cdobratz/AzGuard"
  license "MIT"
  version "1.0.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_darwin_arm64.zip"
      sha256 "85c264a659a4f741b24e4993ef6634da569a719f8d6580aa2e325f6ea66d5c11"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_darwin_amd64.zip"
      sha256 "884bd341c4839a73991d6af14f5ce5ab984a0cfd330fa894841327fc5febe11e"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_linux_arm64.tar.gz"
      sha256 "02022b9385a7dec46244ba893a58a9e8a61d852abca3141453e335194b7fc3d2"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.0.0/azguard_1.0.0_linux_amd64.tar.gz"
      sha256 "0d9463338e81d837649d4f0648cf7d19c5b76eb412627342eae82ebe21d40c3f"
    end
  end

  def install
    bin.install "azguard"
  end

  test do
    system "#{bin}/azguard", "--version"
  end
end
