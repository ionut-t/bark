package enclosing

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// relatedTo returns the labels of the related code for a change of the
// line of file containing change.
func relatedTo(t *testing.T, repo fakeRepo, file, change string) []string {
	t.Helper()
	line := lineOf(t, repo[file], change)
	return labels(findRelated(t, repo, addDiff(repo, file, line, line), RelatedOptions{}))
}

const projectTS = `export interface Project {
  id: string;
  teamMembers?: string[];
}
`

const componentTS = `import { Project } from '../models/project';

export class ProjectComponent {
  project!: Project;

  count(p: Project, label: Label): number {
    return p.teamMembers.length;
  }

  names(): string[] {
    return this.project.teamMembers.map((m) => m.trim());
  }
}
`

func TestRelated_TypeScriptTypes(t *testing.T) {
	repo := fakeRepo{
		"src/models/project.ts":   projectTS,
		"src/models/label.ts":     "export type Label = string;\n",
		"src/app/component.ts":    componentTS,
		"src/app/unrelated.ts":    "export function project() {}\n",
		"src/models/project2.ts":  "export const Project = 1;\n",
		"src/app/members.spec.ts": "describe('Project', () => {});\n",
	}

	tests := []struct {
		name, change string
		want         []string
	}{
		{"a field read through this", "this.project.teamMembers", []string{"src/models/project.ts:1-4 definition of Project"}},
		// label isn't used on the line, so Label isn't shown.
		{"a parameter the line uses", "p.teamMembers.length", []string{"src/models/project.ts:1-4 definition of Project"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, relatedTo(t, repo, "src/app/component.ts", tt.change))
		})
	}
}

func TestRelated_AngularFields(t *testing.T) {
	repo := fakeRepo{
		"src/models/project.ts": projectTS,
		"src/app/store.ts":      "export class ProjectStore {\n  load(): void {}\n}\n",
		"src/app/component.ts": `import { Project } from '../models/project';
import { ProjectStore } from './store';

export class ProjectComponent {
  project = input.required<Project>();
  private store = inject(ProjectStore);

  names(): string[] {
    return this.project().teamMembers.map((m) => m.trim());
  }

  reload(): void {
    this.store.refresh();
  }
}
`,
	}

	assert.Equal(t, []string{"src/models/project.ts:1-4 definition of Project"}, relatedTo(t, repo, "src/app/component.ts", "teamMembers.map"))
	assert.Equal(t, []string{"src/app/store.ts:1-3 definition of ProjectStore"}, relatedTo(t, repo, "src/app/component.ts", "this.store.refresh"))
}

// A variable's type inferred from what a function returns is in the
// function's signature, followed one step further.
func TestRelated_InferredType(t *testing.T) {
	repo := fakeRepo{
		"src/models/project.ts": projectTS,
		"src/app/store.ts":      "import { Project } from '../models/project';\n\nexport function selected(): Project {\n  return load();\n}\n",
		"src/app/component.ts":  "import { selected } from './store';\n\nexport function names() {\n  const p = selected();\n  return p.teamMembers.map((m) => m.trim());\n}\n",
	}

	got := findRelated(t, repo, addDiff(repo, "src/app/component.ts", 4, 5), RelatedOptions{})

	assert.Equal(t, []string{"src/app/store.ts:3-5 definition of selected", "src/models/project.ts:1-4 definition of Project"}, labels(got))
}

// Of a long type, the members the change uses are shown.
func TestRelated_MembersOfLongTypes(t *testing.T) {
	var b strings.Builder
	b.WriteString("export interface Project {\n")
	for i := range 70 {
		fmt.Fprintf(&b, "  field%d: string;\n", i)
	}
	b.WriteString("  teamMembers?: string[];\n}\n")
	repo := fakeRepo{
		"src/models/project.ts": b.String(),
		"src/app/component.ts":  "import { Project } from '../models/project';\n\nexport function names(p: Project) {\n  return p.teamMembers.map((m) => m.trim());\n}\n",
	}
	members := lineOf(t, b.String(), "teamMembers")

	got := relatedTo(t, repo, "src/app/component.ts", "teamMembers")

	assert.Equal(t, []string{fmt.Sprintf("src/models/project.ts:%d-%d teamMembers in Project, lines 1-73", members, members)}, got)
}

const projectPy = `from pydantic import BaseModel, Field, field_validator

from app.base import BaseProject


class Project(BaseProject):
    team_members: list[str] | None = Field(default=None)
`

func TestRelated_Pydantic(t *testing.T) {
	base := "from pydantic import BaseModel\n\n\nclass BaseProject(BaseModel):\n    id: str\n"
	tests := []struct {
		name string
		repo fakeRepo
		file string
		// change is the text of the changed line.
		change string
		want   []string
	}{
		{
			name: "a parameter's annotation, and the model's base",
			repo: fakeRepo{
				"app/models.py": projectPy, "app/base.py": base,
				"app/api.py": "from app.models import Project\n\n\ndef names(project: Project) -> list[str]:\n    return [m.strip() for m in project.team_members]\n",
			},
			file: "app/api.py", change: "project.team_members",
			want: []string{"app/models.py:6-7 definition of Project", "app/base.py:4-5 definition of BaseProject"},
		},
		{
			name: "a model built with model_validate",
			repo: fakeRepo{
				"app/models.py": projectPy, "app/base.py": base,
				"app/api.py": "from app.models import Project\n\n\ndef names(data):\n    p = Project.model_validate(data)\n    return p.team_members\n",
			},
			file: "app/api.py", change: "model_validate",
			want: []string{"app/models.py:6-7 definition of Project", "app/base.py:4-5 definition of BaseProject"},
		},
		{
			name: "a model imported under an alias",
			repo: fakeRepo{
				"app/models.py": projectPy, "app/base.py": base,
				"app/api.py": "from app.models import Project as P\n\n\ndef names(p: P):\n    return p.team_members\n",
			},
			file: "app/api.py", change: "return p.team_members",
			want: []string{"app/models.py:6-7 definition of Project", "app/base.py:4-5 definition of BaseProject"},
		},
		{
			name: "a field set from an annotated parameter",
			repo: fakeRepo{
				"app/repo.py": "class Repo:\n    def load(self):\n        return 1\n",
				"app/svc.py":  "from app.repo import Repo\n\n\nclass Service:\n    def __init__(self, repo: Repo):\n        self.repo = repo\n\n    def run(self):\n        return self.repo.items\n",
			},
			file: "app/svc.py", change: "self.repo.items",
			want: []string{"app/repo.py:1-3 definition of Repo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, relatedTo(t, tt.repo, tt.file, tt.change))
		})
	}
}

// A validator can change a field as much as its type: in a long model, the
// field and its validator are both shown.
func TestRelated_PydanticValidator(t *testing.T) {
	var b strings.Builder
	b.WriteString("class Project(BaseModel):\n")
	for i := range 70 {
		fmt.Fprintf(&b, "    field%d: str\n", i)
	}
	b.WriteString("    team_members: list[str] | None = None\n\n    @field_validator('team_members')\n    @classmethod\n    def default_members(cls, v):\n        return v or []\n")
	src := b.String()
	repo := fakeRepo{
		"app/models.py": src,
		"app/api.py":    "from app.models import Project\n\n\ndef names(p: Project):\n    return p.team_members\n",
	}
	field, validator := lineOf(t, src, "team_members: list"), lineOf(t, src, "@field_validator")

	got := relatedTo(t, repo, "app/api.py", "return p.team_members")

	assert.Equal(t, []string{
		fmt.Sprintf("app/models.py:%d-%d team_members in Project, lines 1-77", field, field),
		fmt.Sprintf("app/models.py:%d-%d team_members in Project, lines 1-77", validator, validator+3),
	}, got)
}

func TestRelated_GoTypes(t *testing.T) {
	project := func(pkg string) string {
		return "package " + pkg + "\n\ntype Project struct {\n\tMembers []string\n}\n"
	}
	tests := []struct {
		name string
		repo fakeRepo
		file string
		// change is the text of the changed line.
		change string
		want   []string
	}{
		{
			name: "a field read through the receiver",
			repo: fakeRepo{
				"models/project.go": project("models"),
				"app/store.go":      "package app\n\ntype Store struct {\n\tp models.Project\n}\n",
				"app/run.go":        "package app\n\nfunc (s *Store) Names() []string {\n\treturn s.p.Members\n}\n",
			},
			file: "app/run.go", change: "s.p.Members",
			want: []string{"app/store.go:3-5 definition of Store", "models/project.go:3-5 definition of Project"},
		},
		{
			name: "a type through an import alias",
			repo: fakeRepo{
				"models/project.go": project("models"),
				"other/project.go":  project("other"),
				"third/project.go":  project("third"),
				"app/run.go":        "package app\n\nimport m \"example.com/x/models\"\n\nfunc Names(p *m.Project) []string {\n\treturn p.Members\n}\n",
			},
			file: "app/run.go", change: "p.Members",
			want: []string{"models/project.go:3-5 definition of Project"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, relatedTo(t, tt.repo, tt.file, tt.change))
		})
	}
}

func TestRelated_RustTypes(t *testing.T) {
	repo := fakeRepo{
		"src/models.rs": "pub struct Project {\n    pub members: Vec<String>,\n}\n\npub struct Team {\n    pub name: String,\n}\n",
		"src/app.rs":    "use crate::models::Project as P;\n\npub struct App {\n    team: Team,\n}\n\nimpl App {\n    pub fn names(&self, p: &P) -> usize {\n        p.members.len() + self.team.name.len()\n    }\n}\n",
	}

	assert.Equal(t, []string{
		"src/app.rs:3-5 definition of App",
		"src/models.rs:1-3 definition of Project",
		"src/models.rs:5-7 definition of Team",
	}, relatedTo(t, repo, "src/app.rs", "p.members.len()"))
}
