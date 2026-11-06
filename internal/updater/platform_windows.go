package updater

// Release asset names. Assets are looked up by exact name, so the release
// process must upload the binary and its detached signature under these.
const (
	AssetName          = "copynote.exe"
	SignatureAssetName = "copynote.exe.sig"
)

// The release asset is the executable itself.
var platformPackaging = plainFile
