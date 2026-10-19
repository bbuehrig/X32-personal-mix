package version

// Version is the semver for this build (no leading "v").
// Release builds may override via:
//
//	-ldflags "-X X32-personal-mix/internal/version.Version=0.2.0"
var Version = "0.2.0"
