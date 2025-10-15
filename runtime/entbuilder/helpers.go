package entbuilder

import (
	"database/sql/driver"

	"entgo.io/ent/schema/field"
)

// SimpleField creates a FieldDescriptor for non-nillable fields without custom value scanning.
// This is the most common case and reduces 15 lines of boilerplate to 1 line.
//
// Example usage:
//   SimpleField(user.FieldAge, field.TypeInt, (*UserMutation).Age, func(n *User, v int) { n.Age = v })
func SimpleField[C, N, M, T any](
	column string,
	typ field.Type,
	getter func(M) (T, bool),
	setter func(*N, T),
) FieldDescriptor[C, N, M] {
	return FieldDescriptor[C, N, M]{
		Column: column,
		Type:   typ,
		Value: func(m M) (FieldValue, bool, error) {
			if value, ok := getter(m); ok {
				return FieldValue{Spec: value, Node: value}, true, nil
			}
			return FieldValue{}, false, nil
		},
		Assign: func(node *N, fv FieldValue) error {
			setter(node, fv.Node.(T))
			return nil
		},
	}
}

// NillableField creates a FieldDescriptor for nillable fields (pointer types).
// This handles the case where the field value needs to be wrapped in a pointer.
//
// Example usage:
//   NillableField(user.FieldNickname, field.TypeString, (*UserMutation).Nickname, func(n *User, v *string) { n.Nickname = v })
func NillableField[C, N, M, T any](
	column string,
	typ field.Type,
	getter func(M) (T, bool),
	setter func(*N, *T),
) FieldDescriptor[C, N, M] {
	return FieldDescriptor[C, N, M]{
		Column: column,
		Type:   typ,
		Value: func(m M) (FieldValue, bool, error) {
			if value, ok := getter(m); ok {
				valueCopy := value
				return FieldValue{Spec: value, Node: &valueCopy}, true, nil
			}
			return FieldValue{}, false, nil
		},
		Assign: func(node *N, fv FieldValue) error {
			if v, ok := fv.Node.(*T); ok {
				setter(node, v)
			}
			return nil
		},
	}
}

// FieldWithScanner creates a FieldDescriptor for fields with custom value scanning logic.
// This is used for types that implement sql.Scanner like UUIDs, custom types, etc.
//
// Example usage:
//   FieldWithScanner(user.FieldID, field.TypeUUID, (*UserMutation).ID,
//     func(v uuid.UUID) (driver.Value, error) { return v, nil },
//     func(n *User, v uuid.UUID) { n.ID = v })
func FieldWithScanner[C, N, M, T any](
	column string,
	typ field.Type,
	getter func(M) (T, bool),
	scanner func(T) (driver.Value, error),
	setter func(*N, T),
) FieldDescriptor[C, N, M] {
	return FieldDescriptor[C, N, M]{
		Column: column,
		Type:   typ,
		Value: func(m M) (FieldValue, bool, error) {
			if value, ok := getter(m); ok {
				scanned, err := scanner(value)
				if err != nil {
					return FieldValue{}, false, err
				}
				return FieldValue{Spec: scanned, Node: value}, true, nil
			}
			return FieldValue{}, false, nil
		},
		Assign: func(node *N, fv FieldValue) error {
			setter(node, fv.Node.(T))
			return nil
		},
	}
}

// NillableFieldWithScanner combines nillable and scanner support.
func NillableFieldWithScanner[C, N, M, T any](
	column string,
	typ field.Type,
	getter func(M) (T, bool),
	scanner func(T) (driver.Value, error),
	setter func(*N, *T),
) FieldDescriptor[C, N, M] {
	return FieldDescriptor[C, N, M]{
		Column: column,
		Type:   typ,
		Value: func(m M) (FieldValue, bool, error) {
			if value, ok := getter(m); ok {
				scanned, err := scanner(value)
				if err != nil {
					return FieldValue{}, false, err
				}
				valueCopy := value
				return FieldValue{Spec: scanned, Node: &valueCopy}, true, nil
			}
			return FieldValue{}, false, nil
		},
		Assign: func(node *N, fv FieldValue) error {
			if v, ok := fv.Node.(*T); ok {
				setter(node, v)
			}
			return nil
		},
	}
}
