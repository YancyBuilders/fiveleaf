cask "fiveleaf" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.1"
  sha256 arm:   "919f1a247c6f2854895567cc785263a65fa4eca39c0789a7350bea0eae576a14",
         intel: "ad504caf21a7de7cc822d7711381f0e3ceb54fd44cf2b188c50325589b36b85b"

  url "https://github.com/YancyBuilders/fiveleaf/releases/download/v#{version}/fiveleaf-#{version}-#{arch}.dmg"
  name "FiveLeaf"
  desc "Small, fast code editor"
  homepage "https://github.com/YancyBuilders/fiveleaf"

  depends_on macos: :ventura

  app "FiveLeaf.app"
  command_wrapper "ff", content: <<~SH
    #!/bin/sh
    # Managed by Homebrew; use brew uninstall --cask fiveleaf to remove it.
    app=#{(appdir/"FiveLeaf.app/Contents/MacOS/FiveLeaf").to_s.shellescape}
    [ -x "$app" ] || { echo "ff: FiveLeaf.app is not found" >&2; exit 1; }
    FIVELEAF_CLI=1 exec "$app" "$@"
  SH
end
