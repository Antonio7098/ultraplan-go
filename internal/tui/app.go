package tui

import (
	"context"
	"fmt"
	"io"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

type Options struct {
	UseCases app.OperationalUseCases
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
	cancel context.CancelFunc
}

func newTeaModel(ctx context.Context, useCases app.OperationalUseCases, width int) teaModel {
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
		case ActionConfirm:
			if m.model.Confirmation != nil && !m.model.Running {
				req := m.model.Confirmation.Request
				opctx, cancel := context.WithCancel(m.ctx)
				m.cancel = cancel
				m.model.Running = true
				m.model.Events = nil
				return m, m.operationCmd(opctx, req)
			}
			return m, nil
		case ActionQuit:
			if m.model.Running && m.cancel != nil {
				m.cancel()
				return m, nil
			}
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, tea.Quit
		case ActionRefresh:
			m.model.Loading = true
			return m, m.refreshCmd()
		case ActionOpen:
			if item, ok := m.model.selectedItem(); ok && item.Validation != nil && m.model.Focus == FocusContent && m.model.Preview == nil {
				m.model.Loading = true
				return m, m.validationCmd(*item.Validation)
			}
			if item, ok := m.model.selectedItem(); ok && item.Operation != nil && m.model.Focus == FocusContent {
				m.model.Loading = true
				return m, m.confirmationCmd(*item.Operation)
			}
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
	case OperationMsg:
		m.model = m.model.Update(v)
		return m, m.refreshCmd()
	case LoadMsg, RefreshMsg, PreviewMsg, ValidationMsg, ConfirmationMsg, OperationEventMsg:
		m.model = m.model.Update(v)
		return m, nil
	default:
		return m, nil
	}
}

func (m teaModel) confirmationCmd(req app.OperationRequest) tea.Cmd {
	route := m.model.currentRoute()
	return func() tea.Msg {
		r, e := m.model.UseCases.PrepareOperation(m.ctx, req)
		return ConfirmationMsg{Result: r, Err: e, Route: route}
	}
}
func (m teaModel) operationCmd(ctx context.Context, req app.OperationRequest) tea.Cmd {
	route := m.model.currentRoute()
	return func() tea.Msg {
		var events []app.OperationEvent
		r, e := m.model.UseCases.RunOperation(ctx, req, func(event app.OperationEvent) {
			events = append(events, event)
			if len(events) > 100 {
				events = events[len(events)-100:]
			}
		})
		return OperationMsg{Result: r, Err: e, Route: route, Events: events}
	}
}

func (m teaModel) validationCmd(req app.ValidationRequest) tea.Cmd {
	route := m.model.currentRoute()
	return func() tea.Msg {
		result, err := m.model.UseCases.Validate(m.ctx, req)
		return ValidationMsg{Result: result, Err: err, Route: route}
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
