package tones

var defaultProfile = ukProfile()

// SetDefaultProfile records the server tone plan for call signalling.
func SetDefaultProfile(p Profile) {
	defaultProfile = p
}

// DefaultProfile returns the configured server tone plan.
func DefaultProfile() Profile {
	return defaultProfile
}
