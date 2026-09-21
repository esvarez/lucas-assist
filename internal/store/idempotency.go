package store

import (
	"crypto/sha256"
	"encoding/hex"
)

// requestHash identifies "the request" an idempotency key was issued for
// (architecture.md §15: "The record stores the request hash and final
// result reference. Reusing a key with a different request is rejected.").
// For AcceptChangeset, the request is fully described by which changeset —
// scoped to its project and user — is being accepted; there's no other
// varying input.
func requestHash(userID, projectID, changesetID string) string {
	sum := sha256.Sum256([]byte(userID + "|" + projectID + "|" + changesetID))
	return hex.EncodeToString(sum[:])
}
