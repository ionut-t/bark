package enclosing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// pyRepo is a src-layout Python package with three classes named Project:
// a model, a schema and a report row, too many for a lookup by name to
// pick from. Packages re-export their modules' names from __init__.py.
func pyRepo(p, content string) fakeRepo {
	repo := fakeRepo{
		"pyproject.toml":                    "[project]\nname = \"taskhub\"\n",
		"src/taskhub/__init__.py":           "",
		"src/taskhub/models/__init__.py":    "from .base import Base\nfrom .project import Project\n",
		"src/taskhub/models/base.py":        "class Base:\n    id: int\n",
		"src/taskhub/models/project.py":     "from datetime import datetime\n\nfrom .base import Base\n\n\nclass Project(Base):\n    name: str\n    archived_at: datetime | None = None\n",
		"src/taskhub/schemas/__init__.py":   "from .project import *\n",
		"src/taskhub/schemas/project.py":    "from pydantic import BaseModel\n\n\nclass Project(BaseModel):\n    team_members: list[str] = []\n",
		"src/taskhub/reporting/__init__.py": "",
		"src/taskhub/reporting/summary.py":  "from dataclasses import dataclass\n\n\n@dataclass\nclass Project:\n    name: str\n    team_size: int\n",
		"src/taskhub/services/__init__.py":  "",
		"src/taskhub/services/projects.py":  "from taskhub.models import Project\n\n\ndef to_out(project: Project) -> dict:\n    return {\"name\": project.name}\n",
	}
	repo[p] = content
	return repo
}

func TestRelated_PythonImports(t *testing.T) {
	model := []string{"src/taskhub/models/project.py:6-8 definition of Project", "src/taskhub/models/base.py:1-2 definition of Base"}
	schema := []string{"src/taskhub/schemas/project.py:4-5 definition of Project"}
	tests := []struct {
		name, path, content string
		want                []string
	}{
		{
			name:    "an absolute import through a package's __init__",
			path:    "src/taskhub/services/feed.py",
			content: "from taskhub.models import Project\n\n\ndef label(project: Project) -> str:\n    return project.archived_at.date().isoformat()\n",
			want:    model,
		},
		{
			name:    "a relative import under an alias, through a star import",
			path:    "src/taskhub/api/routes.py",
			content: "from ..schemas import Project as ProjectOut\n\n\ndef size(out: ProjectOut) -> int:\n    return len(out.team_members)\n",
			want:    schema,
		},
		{
			name:    "a module imported under an alias",
			path:    "src/taskhub/services/feed.py",
			content: "import taskhub.models as m\n\n\ndef label(project: m.Project) -> str:\n    return project.name\n",
			want:    model,
		},
		{
			name:    "a package imported from its parent, beside a same-named class",
			path:    "src/taskhub/reporting/feed.py",
			content: "from .. import models\nfrom .summary import Project\n\n\ndef label(project: models.Project) -> str:\n    return project.name\n",
			want:    model,
		},
		{
			name:    "a class passed as a value",
			path:    "src/taskhub/services/feed.py",
			content: "from taskhub.models import Project\n\n\ndef load(db, project_id):\n    return db.get(Project, project_id)\n",
			want:    model,
		},
		{
			// Project after models. is an attribute, not the Project
			// imported from summary.
			name:    "an attribute named like an imported class",
			path:    "src/taskhub/reporting/feed.py",
			content: "from .. import models\nfrom .summary import Project\n\n\ndef build(n):\n    return models.Project(name=n, Project=n)\n",
			want:    []string{"src/taskhub/models/project.py:6-8 definition of Project", "src/taskhub/models/base.py:1-2 definition of Base"},
		},
		{
			// Two imports bind Project: neither is trusted, and three
			// classes of that name are too many to show.
			name:    "a function-local import of another class of the same name",
			path:    "src/taskhub/services/feed.py",
			content: "from taskhub.models import Project\n\n\ndef label(project: Project) -> str:\n    return project.name\n\n\ndef other():\n    from taskhub.schemas import Project\n\n    return Project\n",
		},
		{
			name:    "an import with a fallback",
			path:    "src/taskhub/services/feed.py",
			content: "try:\n    from taskhub.models import Project\nexcept ImportError:\n    from taskhub.schemas import Project\n\n\ndef label(project: Project) -> str:\n    return project.name\n",
		},
		{
			name:    "the same import twice",
			path:    "src/taskhub/services/feed.py",
			content: "from taskhub.models import Project\n\n\ndef label(project: Project) -> str:\n    return project.name\n\n\ndef other():\n    from taskhub.models import Project\n\n    return Project\n",
			want:    model,
		},
		{
			name:    "a decorated class",
			path:    "src/taskhub/services/feed.py",
			content: "from taskhub.reporting.summary import Project\n\n\ndef label(row: Project) -> str:\n    return row.name\n",
			want:    []string{"src/taskhub/reporting/summary.py:5-7 definition of Project"},
		},
		{
			name:    "a function called through a module alias",
			path:    "src/taskhub/api/routes.py",
			content: "from ..services import projects as project_service\n\n\ndef show(p):\n    return project_service.to_out(p)\n",
			want:    []string{"src/taskhub/services/projects.py:4-5 definition of to_out"},
		},
		{
			name:    "an import only for type checking",
			path:    "src/taskhub/services/feed.py",
			content: "from typing import TYPE_CHECKING\n\nif TYPE_CHECKING:\n    from taskhub.models import Project\n\n\ndef label(project: \"Project\") -> str:\n    return project.name\n",
			want:    model,
		},
		{
			name:    "a test outside the src directory",
			path:    "tests/test_feed.py",
			content: "from taskhub.schemas import Project\n\n\ndef test_size():\n    assert Project().team_members == []\n",
			want:    schema,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := pyRepo(tt.path, tt.content)
			marker := "    return"
			if tt.path == "tests/test_feed.py" {
				marker = "    assert"
			}
			line := lineOf(t, tt.content, marker)
			got := findRelated(t, repo, addDiff(repo, tt.path, line, line), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

// A module outside the repository falls back to the lookup by name.
func TestRelated_PythonThirdPartyImport(t *testing.T) {
	repo := fakeRepo{
		"app/thing.py": "class Thing:\n    size: int\n",
		"app/api.py":   "from somepkg import Thing\n\n\ndef size(t: Thing) -> int:\n    return t.size\n",
	}
	got := findRelated(t, repo, addDiff(repo, "app/api.py", 5, 5), RelatedOptions{})
	assert.Equal(t, []string{"app/thing.py:1-2 definition of Thing"}, labels(got))
}

func TestPyModuleBases(t *testing.T) {
	tests := []struct {
		from, module string
		want         []string
	}{
		{"src/pkg/a/b.py", ".c", []string{"src/pkg/a/c"}},
		{"src/pkg/a/b.py", "..c.d", []string{"src/pkg/c/d"}},
		{"src/pkg/a/b.py", "..", []string{"src/pkg"}},
		{"pkg/b.py", "...c", nil}, // above the repository
		{"src/pkg/a/b.py", "pkg.c", []string{"src/pkg/a/pkg/c", "src/pkg/a/src/pkg/c", "src/pkg/pkg/c", "src/pkg/src/pkg/c", "src/pkg/c", "src/src/pkg/c", "pkg/c"}},
	}
	for _, tt := range tests {
		t.Run(tt.from+" "+tt.module, func(t *testing.T) {
			assert.Equal(t, tt.want, pyModuleBases(tt.from, tt.module))
		})
	}
}
