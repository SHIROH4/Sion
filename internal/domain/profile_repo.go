package domain

// ProfileRepository manages user profile and AI self-profile persistence.
type ProfileRepository interface {
	SaveProfile(key, value string) error
	LoadProfile() *UserProfile
	SaveProfileValue(key, value string)
	LoadProfileValue(key string) string

	SaveSelfProfile(content string) error
	SaveSelfProfileWithSource(content, source string) error
	LoadSelfProfile() string
}
