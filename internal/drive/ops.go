package drive

// RootID returns the provider root id for an account.
func RootID(userID, driveID string) (string, error) {
	provider := ResolveProvider(userID, driveID, "")
	if m := GetMeta(provider); m.RootKey != "" {
		return m.RootKey, nil
	}
	return provider + "_root", nil
}

// ProviderOf returns the resolved provider for an account context.
func ProviderOf(userID, driveID, tokenFrom string) string {
	return ResolveProvider(userID, driveID, tokenFrom)
}
