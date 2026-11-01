#!/bin/sh
# Builds build/CopyNote.app for this Mac and signs it ad hoc.
#
#   sh tools/macbundle/build.sh            # build/CopyNote.app
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
mkdir -p "$app/Contents/MacOS"
(cd "$root" && go build -ldflags "-s -w ${EXTRA_LDFLAGS:-}" -o "$app/Contents/MacOS/CopyNote" .)

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
echo "$app ($version)"
