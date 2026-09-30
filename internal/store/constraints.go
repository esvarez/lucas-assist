package store

// mergeConstraints appends whatever of newOnes isn't already present in
// existing, preserving existing's order and skipping exact-text
// duplicates (#178). AcceptChangeset uses this to fold a changeset's
// disclosed Assumptions — decompose_task's record of choices made on the
// user's behalf, including answered clarification questions — onto the
// project's Constraints so they outlive the one run that produced them,
// instead of only ever shaping that run's own prompt.
func mergeConstraints(existing, newOnes []string) []string {
	if len(newOnes) == 0 {
		return existing
	}

	seen := make(map[string]bool, len(existing))
	for _, c := range existing {
		seen[c] = true
	}

	merged := existing
	for _, c := range newOnes {
		if seen[c] {
			continue
		}
		seen[c] = true
		merged = append(merged, c)
	}
	return merged
}
