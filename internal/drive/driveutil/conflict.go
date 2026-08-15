package driveutil

// Upload conflict policies, aligned with legacy Mnemo's
// resolveProviderPathConflictAction set and the new store/settings
// UploadConflictPolicies capability list. The zero value / empty string / any
// unrecognized value resolves to conflictOverwrite so existing uploads keep
// their legacy overwrite behavior.
const (
	ConflictPolicyOverwrite = "overwrite"
	ConflictPolicyRefuse    = "refuse"
	ConflictPolicyRename    = "rename"
	ConflictPolicySkip      = "skip"
)

// internal enums used by providers.
const (
	ConflictOverwrite = iota
	ConflictRefuse
	ConflictRename
	ConflictSkip
)

// ResolveConflictPolicy maps the policy string to an internal enum. Empty or
// unknown values resolve to overwrite (the historical default).
func ResolveConflictPolicy(policy string) int {
	switch policy {
	case ConflictPolicyRefuse:
		return ConflictRefuse
	case ConflictPolicyRename:
		return ConflictRename
	case ConflictPolicySkip:
		return ConflictSkip
	default:
		return ConflictOverwrite
	}
}
