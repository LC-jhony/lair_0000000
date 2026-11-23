package services

import "strings"

// Availability returns false when a service has no trustworthy distribution
// for that platform, independently of the signed catalog. The reason is a
// stable code the UI can translate.
func Availability(id, platform string) (bool, string) {
	switch id {
	case "typesense":
		if strings.HasPrefix(platform, "windows-") {
			return false, "unsupported-platform"
		}
	case "redis", "valkey":
		if strings.HasPrefix(platform, "windows-") {
			return false, "unsupported-platform"
		}
	}
	return true, ""
}
