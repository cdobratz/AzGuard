class Azguard < Formula
  desc "One command so Azure and AWS free tiers don't surprise you with a bill"
  homepage "https://github.com/cdobratz/AzGuard"
  version "1.2.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.2.0/azguard_1.2.0_darwin_arm64.zip"
      sha256 "135e8edc54ae143938d9a17de20501902b728df18e6344ce22031888f68fc828"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.2.0/azguard_1.2.0_darwin_amd64.zip"
      sha256 "79a91e4af18c35aea302d93e5e983f2fa900123d4eb13e0586545e1f8c2f3de1"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.2.0/azguard_1.2.0_linux_arm64.tar.gz"
      sha256 "da2d6281347bf6ad315faf2b27c6723dff740fa79a603355564c1ea78c445a4a"
    else
      url "https://github.com/cdobratz/AzGuard/releases/download/v1.2.0/azguard_1.2.0_linux_amd64.tar.gz"
      sha256 "236a13bcd8da8db0a03cbe28e00d6107baeb34cb9fb922ea4719617d5aa685af"
    end
  end

  def install
    bin.install "azguard"
  end

  test do
    system bin/"azguard", "--version"
  end
end
