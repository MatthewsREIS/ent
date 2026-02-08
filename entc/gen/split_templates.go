// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import "path/filepath"

var splitTypeTemplates = []TypeTemplate{
	{
		Name:   "split/type/model",
		Format: splitTypef("model.go"),
	},
	{
		Name:   "split/type/query",
		Format: splitTypef("query.go"),
	},
	{
		Name:   "split/type/create",
		Cond:   notView,
		Format: splitTypef("create.go"),
	},
	{
		Name:   "split/type/update",
		Cond:   notView,
		Format: splitTypef("update.go"),
	},
	{
		Name:   "split/type/delete",
		Cond:   notView,
		Format: splitTypef("delete.go"),
	},
	{
		Name:   "split/type/mutation",
		Cond:   notView,
		Format: splitTypef("mutation.go"),
	},
}

func splitTypef(name string) func(*Type) string {
	return func(t *Type) string {
		return filepath.Join("internal", "split", "type", t.PackageDir(), name)
	}
}

func withSplitTypeTemplates(base []TypeTemplate) []TypeTemplate {
	templates := make([]TypeTemplate, 0, len(base)+len(splitTypeTemplates))
	templates = append(templates, base...)
	templates = append(templates, splitTypeTemplates...)
	return templates
}

func withSplitGraphTemplates(base []GraphTemplate) []GraphTemplate {
	templates := make([]GraphTemplate, 0, len(base)+1)
	for _, tmpl := range base {
		if tmpl.Name == "entql" {
			templates = append(templates, GraphTemplate{
				Name:   "split/entql/facade",
				Format: tmpl.Format,
				Skip:   tmpl.Skip,
			})
			continue
		}
		templates = append(templates, tmpl)
	}
	templates = append(templates, GraphTemplate{
		Name:   "split/entql/internal",
		Format: filepath.Join("internal", "split", "entql", "entql.go"),
		Skip: func(g *Graph) bool {
			return !g.featureEnabled(FeatureEntQL)
		},
	})
	return templates
}
