#!/usr/bin/env bash
# Tag a release and point the Homebrew formula in sdgluck/homebrew-tap at it.
# usage: scripts/release.sh 0.1.0
set -euo pipefail

v=${1:?usage: release.sh <version>}
v=${v#v}
tag=v$v
repo=sdgluck/awo
tap=sdgluck/homebrew-tap
formula=Formula/awo.rb

[[ -z $(git status --porcelain) ]] || { echo "release: working tree not clean" >&2; exit 1; }
go test ./...

git tag -a "$tag" -m "awo $tag"
git push origin "$tag"
gh release create "$tag" --repo "$repo" --title "$tag" --generate-notes

url=https://github.com/$repo/archive/refs/tags/$tag.tar.gz
sha=$(curl -fsSL "$url" | shasum -a 256 | cut -d' ' -f1)

body=$(cat <<RUBY
class Awo < Formula
  desc "Per-tmux-window AWS SSO profile switcher"
  homepage "https://github.com/$repo"
  url "$url"
  sha256 "$sha"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}")
  end

  def caveats
    <<~EOS
      Add the shell hook to ~/.zshrc:
        eval "\$(awo init zsh)"
    EOS
  end

  test do
    assert_equal version.to_s, shell_output("#{bin}/awo version").strip
  end
end
RUBY
)

old=$(gh api "repos/$tap/contents/$formula" -q .sha 2>/dev/null || true)
gh api -X PUT "repos/$tap/contents/$formula" \
  -f message="awo $v" \
  -f content="$(printf '%s\n' "$body" | base64)" \
  ${old:+-f sha="$old"} >/dev/null

echo "released $tag; brew upgrade awo (or: brew install sdgluck/tap/awo)"
