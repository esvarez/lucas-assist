package domain

// Task is a unit of work within a project. Tree-shaped via ParentID.
type Task struct {
	ID                 string   `json:"id"`
	ParentID           string   `json:"parent_id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Status             string   `json:"status"`
	Order              int      `json:"order"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

// ProposedTask is the content of a Task before the commit path assigns it
// an ID, order, and status. It's what skills like decompose_task propose.
type ProposedTask struct {
	Title              string   `json:"title" jsonschema:"description=Short imperative task title"`
	Description        string   `json:"description" jsonschema:"description=What done looks like for this task"`
	AcceptanceCriteria []string `json:"acceptance_criteria" jsonschema:"description=Concrete, checkable conditions for calling this task done"`
}
