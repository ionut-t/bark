package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"image/color"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	lightBackground = tea.BackgroundColorMsg{Color: color.White}
	darkBackground  = tea.BackgroundColorMsg{Color: color.Black}
)

// requestsBackgroundColor reports whether cmd, or a command batched in it,
// asks Bubble Tea for the terminal's background colour.
func requestsBackgroundColor(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}

	switch msg := cmd().(type) {
	case tea.BatchMsg:
		return slices.ContainsFunc(msg, requestsBackgroundColor)
	default:
		return msg == tea.RequestBackgroundColor()
	}
}

// newThemeTestModel builds the parts of the app model that New styles up
// front, without the LLM client and git repository New needs.
func newThemeTestModel(task Task) Model {
	s := styles.New(true)

	m := Model{
		width:                80,
		height:               24,
		config:               config.New("", config.Data{}),
		currentView:          viewTasks,
		isDarkMode:           true,
		styles:               s,
		tasks:                newTasksModel(s, true),
		reviewOptions:        newReviewOptionsModel(s, true),
		prDescriptionOptions: newPRDescriptionOptionsModel(s, true),
		branchInput:          newBranchInputModel(""),
		prNumberInput:        newPRNumberInputModel(""),
		selectedTask:         task,
	}
	m.branchInput.setStyles(s)
	m.prNumberInput.setStyles(s)

	return m
}

func TestModel_InitRequestsBackgroundColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model tea.Model
	}{
		{name: "app", model: newThemeTestModel(TaskNone)},
		{name: "actions", model: NewActionsModel(config.New("", config.Data{}))},
		{name: "assets", model: NewAssetsModel(t.TempDir(), AssetReviewer, AssetActionEdit, "true")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.True(t, requestsBackgroundColor(tt.model.Init()),
				"the background colour must be asked for through Bubble Tea, not queried before the program starts")
		})
	}
}

func TestModel_InitKeepsPendingCmd(t *testing.T) {
	t.Parallel()

	m := newThemeTestModel(TaskReview)
	m.pendingCmd = func() tea.Msg { return taskSelectedMsg{task: TaskReview} }

	batch, ok := m.Init()().(tea.BatchMsg)
	require.True(t, ok, "Init should batch the background request with the pending command")

	var msgs []tea.Msg
	for _, cmd := range batch {
		if cmd != nil {
			msgs = append(msgs, cmd())
		}
	}

	assert.Contains(t, msgs, tea.Msg(taskSelectedMsg{task: TaskReview}))
}

func TestModel_BackgroundColorMsgSetsTheme(t *testing.T) {
	t.Parallel()

	t.Run("light background restyles the app", func(t *testing.T) {
		t.Parallel()

		m := newThemeTestModel(TaskNone)
		m.tasks.list.Select(2)

		updated, _ := m.Update(lightBackground)
		got := updated.(Model)

		assert.False(t, got.isDarkMode)
		assert.Equal(t, styles.New(false).Primary.GetForeground(), got.styles.Primary.GetForeground())
		assert.Equal(t, 2, got.tasks.list.Index(), "restyling must keep the list cursor")
		assert.Len(t, got.tasks.list.Items(), len(tasks))
		assert.NotEmpty(t, got.View().Content)
	})

	t.Run("dark background keeps the default", func(t *testing.T) {
		t.Parallel()

		updated, _ := newThemeTestModel(TaskNone).Update(darkBackground)
		got := updated.(Model)

		assert.True(t, got.isDarkMode)
		assert.Equal(t, styles.New(true).Primary.GetForeground(), got.styles.Primary.GetForeground())
	})

	t.Run("actions", func(t *testing.T) {
		t.Parallel()

		updated, _ := NewActionsModel(config.New("", config.Data{})).Update(lightBackground)
		got := updated.(ActionsModel)

		assert.False(t, got.isDarkMode)
		assert.Equal(t, styles.New(false).Primary.GetForeground(), got.styles.Primary.GetForeground())
	})

	t.Run("assets", func(t *testing.T) {
		t.Parallel()

		s := styles.New(true)
		m := AssetsModel{
			list:   newListModel("Select reviewer", []list.Item{item{title: "a"}, item{title: "b"}}, s, true),
			styles: s,
		}
		m.list.Select(1)

		updated, _ := m.Update(lightBackground)
		got := updated.(AssetsModel)

		assert.Equal(t, styles.New(false).Primary.GetForeground(), got.styles.Primary.GetForeground())
		assert.Equal(t, 1, got.list.Index(), "restyling must keep the list cursor")
	})
}

// editor.New asks the terminal for its background colour unless it is told
// the answer, which blocks, so editors are built by newEditor only.
func TestEditorsAreBuiltWithoutQueryingTheTerminal(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		parsed, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "newEditor" {
				return false
			}

			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "editor" && sel.Sel.Name == "New" {
				assert.Failf(t, "editor.New called directly", "%s: use newEditor", fset.Position(call.Pos()))
			}
			return true
		})
	}
}
