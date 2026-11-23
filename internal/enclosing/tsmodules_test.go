package enclosing

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nxRepo is an Nx workspace: apps/web imports @org/models, a library whose
// index re-exports its files. Other libraries declare a Project of their
// own, too many for a lookup by name to pick from.
func nxRepo(component string) fakeRepo {
	repo := fakeRepo{
		"tsconfig.base.json": `{
  // Path aliases for the libraries.
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@org/models": ["libs/models/src/index.ts"],
      "@org/*": ["libs/*/src/index.ts"], // the rest
    },
  },
}
`,
		"apps/web/tsconfig.json":                "{ \"extends\": \"../../tsconfig.base.json\" }\n",
		"libs/models/src/index.ts":              "export * from './lib/team';\nexport * from './lib/project';\nexport { ProjectModel as Renamed } from './lib/model';\n",
		"libs/models/src/lib/team.ts":           "export interface Team {\n  name: string;\n}\n",
		"libs/models/src/lib/project.ts":        projectTS,
		"libs/models/src/lib/model.ts":          "export interface ProjectModel {\n  id: string;\n}\n",
		"apps/web/src/app/project.component.ts": component,
	}
	for i := range 3 {
		repo[fmt.Sprintf("libs/other%d/src/project.ts", i)] = projectTS
	}
	return repo
}

func TestRelated_TypeScriptImports(t *testing.T) {
	tests := []struct {
		name      string
		component string
		want      []string
	}{
		{
			name:      "a path alias and a barrel",
			component: "import { Project } from '@org/models';\n\nexport function names(p: Project) {\n  return p.teamMembers.map((m) => m);\n}\n",
			want:      []string{"libs/models/src/lib/project.ts:1-4 definition of Project"},
		},
		{
			name:      "an alias, on the import and in the barrel",
			component: "import { Renamed as R } from '@org/models';\n\nexport function names(p: R) {\n  return p.teamMembers.map((m) => m);\n}\n",
			want:      []string{"libs/models/src/lib/model.ts:1-3 definition of ProjectModel"},
		},
		{
			name:      "a namespace import",
			component: "import * as models from '@org/models';\n\nexport function names(p: models.Project) {\n  return p.teamMembers.map((m) => m);\n}\n",
			want:      []string{"libs/models/src/lib/project.ts:1-4 definition of Project"},
		},
		{
			name:      "a wildcard path alias",
			component: "import { Project } from '@org/models';\nimport { Team } from '@org/teams';\n\nexport function names(p: Project, t: Team) {\n  return t.name;\n}\n",
			want:      []string{"libs/teams/src/lib/team.ts:1-3 definition of Team"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := nxRepo(tt.component)
			repo["libs/teams/src/index.ts"] = "export { Team } from './lib/team';\n"
			repo["libs/teams/src/lib/team.ts"] = "export interface Team {\n  name: string;\n}\n"
			line := lineOf(t, tt.component, "return")
			got := findRelated(t, repo, addDiff(repo, "apps/web/src/app/project.component.ts", line, line), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

func TestRelated_TypeScriptRelativeImports(t *testing.T) {
	tests := []struct {
		name string
		repo fakeRepo
		want []string
	}{
		{
			name: "a default export",
			repo: fakeRepo{
				"src/def.ts": "export default interface Thing {\n  a: number;\n}\n",
				"src/app.ts": "import Def from './def';\n\nexport function f(d: Def) {\n  return d.a;\n}\n",
			},
			want: []string{"src/def.ts:1-3 definition of Thing"},
		},
		{
			name: "an ESM import of the compiled file",
			repo: fakeRepo{
				"src/models/project.ts": projectTS,
				"src/app.ts":            "import type { Project } from './models/project.js';\n\nexport function f(d: Project) {\n  return d.teamMembers;\n}\n",
			},
			want: []string{"src/models/project.ts:1-4 definition of Project"},
		},
		{
			name: "a directory's index re-exporting an import",
			repo: fakeRepo{
				"src/models/project.ts": projectTS,
				"src/models/index.ts":   "import { Project } from './project';\n\nexport { Project };\n",
				"src/app.ts":            "import { Project } from './models';\n\nexport function f(d: Project) {\n  return d.teamMembers;\n}\n",
			},
			want: []string{"src/models/project.ts:1-4 definition of Project"},
		},
		{
			// The type is inferred from a schema: the type, then the schema.
			name: "a zod schema",
			repo: fakeRepo{
				"src/schema.ts": "import { z } from 'zod';\n\nexport const ProjectSchema = z.object({\n  teamMembers: z.array(z.string()).optional(),\n});\n\nexport type Project = z.infer<typeof ProjectSchema>;\n",
				"src/app.ts":    "import { Project } from './schema';\n\nexport function f(d: Project) {\n  return d.teamMembers;\n}\n",
			},
			want: []string{"src/schema.ts:7-7 definition of Project", "src/schema.ts:3-5 definition of ProjectSchema"},
		},
		{
			name: "a package outside the repository, by name",
			repo: fakeRepo{
				"src/project.ts": projectTS,
				"src/app.ts":     "import { Project } from 'some-package';\n\nexport function f(d: Project) {\n  return d.teamMembers;\n}\n",
			},
			want: []string{"src/project.ts:1-4 definition of Project"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := lineOf(t, tt.repo["src/app.ts"], "return")
			got := findRelated(t, tt.repo, addDiff(tt.repo, "src/app.ts", line, line), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

func TestTsPathsCandidates(t *testing.T) {
	c := tsPaths{base: "root", baseURL: "root", paths: map[string][]string{
		"@org/*":      {"libs/*/src/index.ts"},
		"@org/ui/*":   {"libs/ui/*/index.ts"},
		"@org/models": {"libs/models/src/index.ts"},
	}}

	assert.Equal(t, []string{"root/libs/models/src/index.ts", "root/@org/models"}, c.candidates("@org/models"))
	assert.Equal(t, []string{"root/libs/ui/button/index.ts", "root/@org/ui/button"}, c.candidates("@org/ui/button"), "the longest prefix wins")
	assert.Equal(t, []string{"root/libs/teams/src/index.ts", "root/@org/teams"}, c.candidates("@org/teams"))
	assert.Equal(t, []string{"root/rxjs"}, c.candidates("rxjs"))
}

func TestStripJSONC(t *testing.T) {
	src := `{
  // a comment
  "a": "http://x/*not a comment*/", /* a block
  comment */ "b": [1, 2,],
  "c": "quote \" // still a string", // a comma before a comment
}`

	var got map[string]any
	require.NoError(t, json.Unmarshal(stripJSONC([]byte(src)), &got))

	assert.Equal(t, map[string]any{"a": "http://x/*not a comment*/", "b": []any{1.0, 2.0}, "c": "quote \" // still a string"}, got)
}
