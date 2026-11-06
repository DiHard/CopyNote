#!/bin/sh
# Builds build/CopyNote.app for Apple silicon and Intel Macs in one
# universal binary, signs it ad hoc, and zips it as the release asset.
#
#   sh tools/macbundle/build.sh            # build/CopyNote.app + CopyNote-macos.zip
#   OUT=/tmp/x sh tools/macbundle/build.sh # somewhere else
#
# EXTRA_LDFLAGS is appended to the linker flags, for the -X overrides of a
# test build. A bundle built on this machine carries no quarantine flag, so
# Gatekeeper opens it without asking; the ad hoc signature is what macOS
# needs to run an arm64 binary at all.
set -eu

root=$(cd "$(dirname "$0")/../.." && pwd)
out=${OUT:-"$root/build"}
app="$out/CopyNote.app"
version=$(sed -n 's/^var Version = "\(.*\)"$/\1/p' "$root/internal/version/version.go")
[ -n "$version" ] || { echo "version not found in internal/version/version.go" >&2; exit 1; }

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
# One binary per architecture, glued into one with lipo. cgo builds for the
# other architecture with the same Xcode clang; nothing else is needed.
for arch in arm64 amd64; do
	(cd "$root" && CGO_ENABLED=1 GOARCH=$arch go build -trimpath -ldflags "-s -w ${EXTRA_LDFLAGS:-}" -o "$out/CopyNote-$arch" .)
done
lipo -create -output "$app/Contents/MacOS/CopyNote" "$out/CopyNote-arm64" "$out/CopyNote-amd64"
rm -f "$out/CopyNote-arm64" "$out/CopyNote-amd64"

# The icon is drawn by the same generator as the Windows .ico, so the glyph
# lives in one place; only the .icns is kept, beside the binary.
iconset="$out/CopyNote.iconset"
rm -rf "$iconset"
(cd "$root" && go run tools/genicon/main.go -macos "$iconset")
iconutil -c icns "$iconset" -o "$app/Contents/Resources/CopyNote.icns"
rm -rf "$iconset"

cat > "$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>dev.copynote.app</string>
	<key>CFBundleName</key>
	<string>CopyNote</string>
	<key>CFBundleDisplayName</key>
	<string>CopyNote</string>
	<key>CFBundleExecutable</key>
	<string>CopyNote</string>
	<key>CFBundleIconFile</key>
	<string>CopyNote</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>$version</string>
	<key>CFBundleVersion</key>
	<string>$version</string>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<!-- A menu bar app: no Dock icon, no entry in the app switcher. -->
	<key>LSUIElement</key>
	<true/>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleLocalizations</key>
	<array>
		<string>en</string>
		<string>ru</string>
	</array>
</dict>
</plist>
PLIST

codesign --force --sign - "$app"

# The release asset. ditto, not zip: it keeps the signature's files and the
# executable bit exactly as they are, and a bundle whose signature breaks on
# the way is one macOS calls damaged rather than unverified.
zip="$out/CopyNote-macos.zip"
rm -f "$zip"
ditto -c -k --keepParent "$app" "$zip"
echo "$app ($version)"
echo "$zip"
