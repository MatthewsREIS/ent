package entgen

// FieldRequirement describes whether a field is required during create or update
// operations and provides the error factory for reporting missing values.
type FieldRequirement struct {
	// Required indicates that the field must be provided for all dialects.
	Required bool
	// Dialects restricts the requirement to the listed SQL dialects. When empty,
	// the requirement applies to none of them (use Required instead).
	Dialects map[string]struct{}
	// Error is called when the requirement fails and should construct the error
	// returned to the caller.
	Error func() error
}

// Applies reports whether the requirement should be enforced for the given dialect.
func (r FieldRequirement) Applies(dialect string) bool {
	if r.Error == nil {
		return false
	}
	if r.Required {
		return true
	}
	if len(r.Dialects) == 0 {
		return false
	}
	_, ok := r.Dialects[dialect]
	return ok
}

// FieldSpec holds metadata and callbacks for a field mutation.
type FieldSpec[M any] struct {
	Name string
	// Requirement defines when the field must be provided.
	Requirement FieldRequirement
	// Default assigns the default value to the field (when missing).
	Default func(M) error
	// IsSet reports whether the field exists on the mutation object.
	IsSet func(M) bool
	// Validators hold field-specific validation callbacks. They should be no-ops
	// when the value is not set.
	Validators []func(M) error
}

// EdgeRequirement describes whether an edge must be provided.
type EdgeRequirement struct {
	Required bool
	Error    func() error
}

// EdgeSpec holds metadata for edge mutations (relations) used during validation.
type EdgeSpec[M any] struct {
	Name        string
	Requirement EdgeRequirement
	// Count returns how many IDs were attached to the mutation for this edge.
	Count func(M) int
}

// CreateSpec aggregates metadata for validating a create mutation.
type CreateSpec[M any] struct {
	Fields []FieldSpec[M]
	Edges  []EdgeSpec[M]
}

// ApplyDefaults executes all default handlers registered for the mutation.
func ApplyDefaults[M any](mutation M, fields []FieldSpec[M]) error {
	for _, field := range fields {
		if field.Default == nil {
			continue
		}
		if err := field.Default(mutation); err != nil {
			return err
		}
	}
	return nil
}

// CheckCreate runs validations on the given mutation according to the provided specification.
func CheckCreate[M any](dialect string, mutation M, spec CreateSpec[M]) error {
	for _, field := range spec.Fields {
		if field.Requirement.Applies(dialect) && field.IsSet != nil && !field.IsSet(mutation) {
			return field.Requirement.Error()
		}
		for _, validate := range field.Validators {
			if validate == nil {
				continue
			}
			if err := validate(mutation); err != nil {
				return err
			}
		}
	}
	for _, edge := range spec.Edges {
		if !edge.Requirement.Required || edge.Count == nil {
			continue
		}
		if edge.Count(mutation) == 0 {
			return edge.Requirement.Error()
		}
	}
	return nil
}
