package agent

import "fmt"

// Registry maps skill name -> Skill implementation.
type Registry struct {
	skills map[string]Skill
}

// NewRegistry builds a registry from a fixed set of skills. Adding a skill
// means adding it to this list, not new dispatch plumbing.
func NewRegistry(skills ...Skill) *Registry {
	r := &Registry{skills: make(map[string]Skill, len(skills))}
	for _, s := range skills {
		r.skills[s.Name()] = s
	}
	return r
}

// Get looks up a skill by name.
func (r *Registry) Get(name string) (Skill, error) {
	s, ok := r.skills[name]
	if !ok {
		return nil, fmt.Errorf("unknown skill %q", name)
	}
	return s, nil
}
