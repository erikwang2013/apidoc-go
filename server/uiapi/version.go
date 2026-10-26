package uiapi

import (
	"runtime/debug"
	"strings"
)

const modulePath = "github.com/erikwang2013/apidoc-go"

// moduleVersion returns the apidoc-go module version when it can be read from
// Go's build info (dependency mode, or a tagged main module). Development
// builds fall back to "dev".
func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if version := cleanVersion(info.Main.Version); info.Main.Path == modulePath && version != "" {
		return version
	}
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		if dep.Replace != nil {
			if version := cleanVersion(dep.Replace.Version); version != "" {
				return version
			}
			continue
		}
		if version := cleanVersion(dep.Version); version != "" {
			return version
		}
	}
	return "dev"
}

func cleanVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	if version == "" || version == "(devel)" {
		return ""
	}
	return version
}
