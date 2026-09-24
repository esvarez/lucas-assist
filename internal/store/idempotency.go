package store

import (
	"crypto/sha256"
	"encoding/hex"
)

// requestHash identifies "the request" an idempotency key was issued for
// (architecture.md §15: "The record stores the request hash and final
// result reference. Reusing a key with a different request is rejected.").
// For AcceptCreateProjectChangeset, the request is fully described by
// which changeset — scoped to its project and user — is being accepted;
// there's no other varying input. AcceptChangeset uses
// requestHashWithFingerprint instead (see its doc comment for why).
func requestHash(userID, projectID, changesetID string) string {
	sum := sha256.Sum256([]byte(userID + "|" + projectID + "|" + changesetID))
	return hex.EncodeToString(sum[:])
}

// requestHashWithFingerprint is AcceptChangeset's variant of requestHash
// (#169): a changeset can now be accepted more than once over its
// lifetime — one call per partial batch, with the rest left "proposed" —
// so "the request" also depends on which proposed tasks this specific
// call selected.
//
// fingerprint has to represent exactly what the *client's request body*
// asked for (the caller passes a marshaled form of it), not the
// changeset's proposed_tasks as resolved against current server state —
// those shrink after every accept, so hashing the resolved list would
// make an identical replay of the very same request hash differently
// once the first attempt already succeeded, wrongly rejecting a
// legitimate retry as a reused key instead of returning the cached
// result.
func requestHashWithFingerprint(userID, projectID, changesetID, fingerprint string) string {
	sum := sha256.Sum256([]byte(userID + "|" + projectID + "|" + changesetID + "|" + fingerprint))
	return hex.EncodeToString(sum[:])
}
