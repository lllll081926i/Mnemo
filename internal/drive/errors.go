package drive

import "errors"

// Sentinel errors returned across the drive facade.
var (
	// ErrUnknownProvider means the account could not be mapped to a registered provider.
	ErrUnknownProvider = errors.New("drive: unknown provider")
	// ErrNotImplemented means the provider does not implement an optional capability.
	ErrNotImplemented = errors.New("drive: capability not implemented")
	// ErrUnauthorized means the session token is missing or expired.
	ErrUnauthorized = errors.New("drive: unauthorized")
	// ErrNotFound means the target file/folder does not exist.
	ErrNotFound = errors.New("drive: not found")
)

// NotSupported returns a capability-gated error for a driver method.
func NotSupported(what string) error {
	return errors.New("drive: " + what + " not supported by this provider")
}
