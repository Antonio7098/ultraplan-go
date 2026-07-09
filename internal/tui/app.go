package tui

import (
	"context"
	"fmt"
	"io"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	UseCases app.ReadOnlyUseCases
	Stdout   io.Writer
	Width    int
}

func Run(ctx context.Context, opts Options) error {
	if opts.UseCases == nil {
		return fmt.Errorf("tui: missing read-only use cases")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	program := tea.NewProgram(newTeaModel(ctx, opts.UseCases, opts.Width), tea.WithAltScreen(), tea.WithOutput(opts.Stdout))
	_, err := program.Run()
	return err
}

type teaModel struct {
	ctx    context.Context
	model  Model
	width  int
	height int
}

func newTeaModel(ctx context.Context, useCases app.ReadOnlyUseCases, width int) teaModel {
	if width <= 0 {
		width = 100
	}
	return teaModel{ctx: ctx, model: NewModel(useCases), width: width, height: 40}
}

func (m teaModel) Init() tea.Cmd {
	return m.loadCmd()
}

func (m teaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		return m, nil
	case tea.KeyMsg:
		action := KeyToAction(v.String())
		switch action {
		case ActionQuit:
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, tea.Quit
		case ActionRefresh:
			m.model.Loading = true
			return m, m.refreshCmd()
		case ActionOpen:
			if item, ok := m.model.selectedItem(); ok && item.Path != "" && m.model.Focus == FocusContent && m.model.Preview == nil {
				return m, m.previewCmd()
			}
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, nil
		case ActionBack, ActionFocusNext, ActionLeft, ActionRight:
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, nil
		case ActionClosePreview:
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, nil
		default:
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, nil
		}
	case LoadMsg, RefreshMsg, PreviewMsg:
		m.model = m.model.Update(v)
		return m, nil
	default:
		return m, nil
	}
}

func (m teaModel) View() string {
	return RenderWithSize(m.model, m.width, m.height)
}

func (m teaModel) loadCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := m.model.UseCases.Dashboard(m.ctx)
		return LoadMsg{Result: result, Err: err}
	}
}

func (m teaModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := m.model.UseCases.Dashboard(m.ctx)
		return RefreshMsg{Result: result, Err: err}
	}
}

func (m teaModel) previewCmd() tea.Cmd {
	return func() tea.Msg {
		item, ok := m.model.selectedItem()
		if !ok || item.Path == "" {
			return PreviewMsg{Result: app.ArtifactPreviewResult{Error: "no previewable artifact selected"}, Route: m.model.currentRoute(), Title: "Preview"}
		}
		result, err := m.model.UseCases.PreviewArtifact(m.ctx, item.Path)
		return PreviewMsg{Result: result, Err: err, Route: m.model.currentRoute(), Title: item.Label}
	}
}
