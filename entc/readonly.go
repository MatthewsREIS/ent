// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entc

import "entgo.io/ent/schema"

// readOnlyAnnotation is the carrier type for the ReadOnly schema annotation.
// Exported via the ReadOnly() constructor; users do not construct it directly.
// The Name() method returns "ReadOnly" so templates can key on it via
// $.Annotations["ReadOnly"] (Type.Annotations is map[string]any).
type readOnlyAnnotation struct{}

// Name implements schema.Annotation.
func (readOnlyAnnotation) Name() string { return "ReadOnly" }

// ReadOnly returns a schema annotation that suppresses generation of update
// and delete builders for the annotated entity, regardless of the global
// FeatureNoUpdate / FeatureNoDelete flags. Use it like:
//
//	func (User) Annotations() []schema.Annotation {
//	    return []schema.Annotation{entc.ReadOnly()}
//	}
//
// The annotation has no runtime semantics — it only affects codegen output.
func ReadOnly() schema.Annotation {
	return readOnlyAnnotation{}
}
