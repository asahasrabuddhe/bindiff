package bindiff

import "errors"

// These errors came from an internal seek buffer that Diff no longer uses.
// Nothing in this package returns them.
//
//nolint:revive,staticcheck // Names kept for compatibility with v0.1.x.
var (
	// Deprecated: Not returned by any function.
	InvalidWhence = errors.New("bindiff: invalid whence")
	// Deprecated: Not returned by any function.
	NegativePosition = errors.New("bindiff: negative position")
	// Deprecated: Not returned by any function.
	PositionOutOfRange = errors.New("bindiff: position out of range")
)
