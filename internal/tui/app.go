package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

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
	ctx         context.Context
	model       Model
	width       int
	height      int
	cancel      context.CancelFunc
	eventStream <-chan app.OperationEvent
}
type runViewTickMsg struct{}

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
		if m.model.ParallelForm != nil {
			switch v.String() {
			case "esc":
				m.model.ParallelForm = nil
				m.model.ParallelValue = ""
				m.model.ParallelError = ""
				return m, nil
			case "backspace":
				if len(m.model.ParallelValue) > 0 {
					m.model.ParallelValue = m.model.ParallelValue[:len(m.model.ParallelValue)-1]
				}
				m.model.ParallelError = ""
				return m, nil
			case "enter":
				value := 3
				if m.model.ParallelValue != "" {
					parsed, err := strconv.Atoi(m.model.ParallelValue)
					if err != nil || parsed < 1 || parsed > 64 {
						m.model.ParallelError = "parallelism must be between 1 and 64"
						return m, nil
					}
					value = parsed
				}
				req := *m.model.ParallelForm
				req.Parallelism = value
				m.model.ParallelForm = nil
				m.model.ParallelValue = ""
				m.model.Loading = true
				return m, m.confirmationCmd(req)
			default:
				if len(v.Runes) == 1 && v.Runes[0] >= '0' && v.Runes[0] <= '9' {
					m.model.ParallelValue += string(v.Runes[0])
					m.model.ParallelError = ""
				}
				return m, nil
			}
		}
		action := KeyToAction(v.String())
		switch action {
		case ActionConfirm:
			if m.model.Confirmation != nil && !m.model.Running {
				return m.beginOperation(m.model.Confirmation.Request)
			}
			return m, nil
		case ActionCancel:
			if m.model.Running && m.model.ActiveRunID != "" {
				if runs, ok := m.model.UseCases.(app.RunUseCases); ok {
					if _, _, err := runs.CancelRun(m.ctx, app.RunID(m.model.ActiveRunID), "user_requested"); err != nil {
						m.model.Error = err.Error()
					} else if m.model.Operation != nil {
						m.model.Operation.Message = "durable cancellation requested; waiting for owner acknowledgement"
					}
					return m, nil
				}
			}
			if m.model.Running && m.cancel != nil {
				m.cancel()
				return m, nil
			}
			if route := m.model.currentRoute(); route.Kind == RouteRun && route.RunID != "" {
				if runs, ok := m.model.UseCases.(app.RunUseCases); ok {
					if _, _, err := runs.CancelRun(m.ctx, app.RunID(route.RunID), "user_requested"); err != nil {
						m.model.Error = err.Error()
					} else {
						m.model.Error = "durable cancellation requested"
					}
					return m, m.refreshCmd()
				}
			}
			study := m.model.RunViewStudy
			if study == "" {
				if item, ok := m.model.selectedItem(); ok {
					study = item.ViewRun
				}
			}
			if study != "" && !m.model.Running {
				return m.beginOperation(app.OperationRequest{Kind: app.OperationStudyCancel, Study: study})
			}
			return m, nil
		case ActionQuit:
			if m.model.Running {
				m.model.OperationHidden = true
				m.model.Error = "active durable work was not cancelled; press c to request cancellation or reopen its run"
				return m, nil
			}
			m.model = m.model.Update(KeyMsg(v.String()))
			return m, tea.Quit
		case ActionRefresh:
			m.model.Loading = true
			return m, m.refreshCmd()
		case ActionOpen:
			if m.model.Confirmation != nil && !m.model.Running {
				return m.beginOperation(m.model.Confirmation.Request)
			}
			if m.model.RunViewStudy != "" {
				m.model.RunViewShowPrevious = !m.model.RunViewShowPrevious
				return m, nil
			}
			if m.model.Running && !m.model.OperationHidden && (m.model.ActiveOperation.Kind == app.OperationStudyStart || m.model.ActiveOperation.Kind == app.OperationStudyResume) {
				m.model.OperationShowPrevious = !m.model.OperationShowPrevious
				return m, nil
			}
			if item, ok := m.model.selectedItem(); ok && item.ViewRun != "" && m.model.Focus == FocusContent {
				m.model.RunViewStudy = item.ViewRun
				m.model.RunViewShowPrevious = false
				return m, tea.Batch(m.refreshCmd(), runViewTickCmd())
			}
			if item, ok := m.model.selectedItem(); ok && item.Validation != nil && m.model.Focus == FocusContent && m.model.Preview == nil {
				m.model.Loading = true
				return m, m.validationCmd(*item.Validation)
			}
			if item, ok := m.model.selectedItem(); ok && item.Operation != nil && m.model.Focus == FocusContent {
				if item.Operation.Kind == app.OperationStudyResume || item.Operation.Kind == app.OperationStudyStart {
					req := *item.Operation
					m.model.ParallelForm = &req
					m.model.ParallelValue = ""
					m.model.ParallelError = ""
					return m, nil
				}
				m.model.Loading = true
				return m, m.confirmationCmd(*item.Operation)
			}
			if item, ok := m.model.selectedItem(); ok && item.Path != "" && m.model.Focus == FocusContent && m.model.Preview == nil {
				return m, m.previewCmd()
			}
			m.model = m.model.Update(KeyMsg(v.String()))
			if m.model.currentRoute().Kind == RouteRun {
				return m, tea.Batch(m.refreshCmd(), runViewTickCmd())
			}
			return m, nil
		case ActionBack, ActionFocusNext, ActionLeft, ActionRight:
			if action == ActionBack && m.model.Running && !m.model.OperationHidden && (m.model.ActiveOperation.Kind == app.OperationStudyStart || m.model.ActiveOperation.Kind == app.OperationStudyResume) {
				m.model.OperationHidden = true
				m.model.OperationShowPrevious = false
				return m, m.refreshCmd()
			}
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
		m.cancel = nil
		m.eventStream = nil
		return m, m.refreshCmd()
	case OperationEventMsg:
		m.model = m.model.Update(v)
		if m.eventStream != nil && m.model.Running {
			return m, waitOperationEvent(m.eventStream)
		}
		return m, nil
	case LoadMsg, RefreshMsg, PreviewMsg, ValidationMsg, ConfirmationMsg:
		m.model = m.model.Update(v)
		return m, nil
	case runViewTickMsg:
		if m.model.RunViewStudy != "" || m.model.currentRoute().Kind == RouteRun {
			return m, tea.Batch(m.refreshCmd(), runViewTickCmd())
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m teaModel) beginOperation(req app.OperationRequest) (tea.Model, tea.Cmd) {
	opctx, cancel := context.WithCancel(m.ctx)
	acceptedRunID := ""
	if manager, ok := m.model.UseCases.(app.DurableOperationManager); ok && m.model.Confirmation != nil {
		basis := m.model.Confirmation.CanonicalRequest + "\x00" + m.model.Confirmation.InputFingerprint
		digest := sha256.Sum256([]byte(basis))
		accepted, err := manager.AcceptOperation(opctx, *m.model.Confirmation, hex.EncodeToString(digest[:]))
		if err != nil {
			cancel()
			m.model.Error = err.Error()
			m.model.Loading = false
			return m, nil
		}
		if accepted.Existing {
			cancel()
			m.model.Operation = &app.OperationResult{State: app.OperationState(accepted.Lifecycle), RunID: accepted.RunID, Message: "matching durable operation already exists"}
			m.model.Confirmation = nil
			return m, nil
		}
		acceptedRunID = accepted.RunID
		if accepted.Context != nil {
			opctx = accepted.Context
		}
	}
	m.cancel = cancel
	m.model.Running = true
	m.model.Events = nil
	m.model.Confirmation = nil
	m.model.RunViewStudy = ""
	m.model.ActiveOperation = req
	m.model.OperationShowPrevious = false
	m.model.OperationHidden = false
	m.model.ActiveRunID = acceptedRunID
	m.model.Operation = &app.OperationResult{State: app.OperationRunning, RunID: acceptedRunID, Subject: operationSubject(req), Message: "operation running; c requests durable cancellation; q keeps the run active"}
	stream := make(chan app.OperationEvent, 128)
	m.eventStream = stream
	return m, tea.Batch(m.operationCmd(opctx, req, acceptedRunID, stream), waitOperationEvent(stream))
}

func runViewTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return runViewTickMsg{} })
}

func (m teaModel) confirmationCmd(req app.OperationRequest) tea.Cmd {
	route := m.model.currentRoute()
	return func() tea.Msg {
		r, e := m.model.UseCases.PrepareOperation(m.ctx, req)
		return ConfirmationMsg{Result: r, Err: e, Route: route}
	}
}
func (m teaModel) operationCmd(ctx context.Context, req app.OperationRequest, runID string, stream chan app.OperationEvent) tea.Cmd {
	route := m.model.currentRoute()
	return func() tea.Msg {
		defer close(stream)
		r, e := m.model.UseCases.RunOperation(ctx, req, func(event app.OperationEvent) {
			if manager, ok := m.model.UseCases.(app.DurableOperationManager); ok && runID != "" {
				committed, err := manager.RecordOperationEvent(context.Background(), runID, event)
				if err != nil && !errors.Is(err, app.ErrWebUnavailable) {
					return
				}
				if err == nil && !committed {
					return
				}
			}
			select {
			case stream <- event:
			default:
			}
		})
		r.RunID = runID
		if manager, ok := m.model.UseCases.(app.DurableOperationManager); ok && runID != "" {
			finishCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			finishErr := manager.FinishOperation(finishCtx, runID, r.State, e)
			cancel()
			if finishErr != nil {
				e = errors.Join(e, finishErr)
			}
		}
		return OperationMsg{Result: r, Err: e, Route: route}
	}
}

func waitOperationEvent(stream <-chan app.OperationEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-stream
		if !ok {
			return nil
		}
		return OperationEventMsg{Event: event}
	}
}

func operationSubject(req app.OperationRequest) string {
	if req.Study != "" {
		return req.Study
	}
	return req.Project + "/" + req.Sprint
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
		result, runs, events, err := m.dashboardAndRuns()
		return LoadMsg{Result: result, Runs: runs, Events: events, Err: err}
	}
}

func (m teaModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		result, runs, events, err := m.dashboardAndRuns()
		return RefreshMsg{Result: result, Runs: runs, Events: events, Err: err}
	}
}

func (m teaModel) dashboardAndRuns() (app.DashboardResult, []app.RunSnapshot, []app.RunEvent, error) {
	result, err := m.model.UseCases.Dashboard(m.ctx)
	if err != nil {
		return result, nil, nil, err
	}
	runsCapability, ok := m.model.UseCases.(app.RunUseCases)
	if !ok {
		return result, nil, nil, nil
	}
	page, err := runsCapability.Runs(m.ctx, app.RunQuery{Limit: 200})
	if err != nil && !errors.Is(err, app.ErrWebUnavailable) {
		return result, nil, nil, err
	}
	var events []app.RunEvent
	route := m.model.currentRoute()
	if route.Kind == RouteRun && route.RunID != "" {
		for _, snapshot := range page.Runs {
			if string(snapshot.RunID) != route.RunID {
				continue
			}
			after := uint64(0)
			if snapshot.OldestRetainedSequence > 1 {
				after = snapshot.OldestRetainedSequence - 1
			}
			events, err = runsCapability.RunEvents(m.ctx, snapshot.RunID, after, 200)
			if err != nil {
				return result, page.Runs, nil, err
			}
			break
		}
	}
	return result, page.Runs, events, nil
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
