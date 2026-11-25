cask "fiveleaf" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.0"
  sha256 arm:   "ab707bcd5096476e0c791296f57cbe11e00316fd31e1ac50d162676992afbf3d",
         intel: "1117fb3c3081e1430694751f49237807ac838a62cb285e937a120841f2450e51"

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
