package tui

type KeyAction string

const (
	ActionNone         KeyAction = "none"
	ActionQuit         KeyAction = "quit"
	ActionUp           KeyAction = "up"
	ActionDown         KeyAction = "down"
	ActionProjects     KeyAction = "projects"
	ActionStudies      KeyAction = "studies"
	ActionRefresh      KeyAction = "refresh"
	ActionOpen         KeyAction = "open"
	ActionClosePreview KeyAction = "close-preview"
	ActionFocusNext    KeyAction = "focus-next"
	ActionBack         KeyAction = "back"
	ActionLeft         KeyAction = "left"
	ActionRight        KeyAction = "right"
	ActionConfirm      KeyAction = "confirm"
)

func KeyToAction(key string) KeyAction {
	switch key {
	case "q", "ctrl+c":
		return ActionQuit
	case "esc", "backspace":
		return ActionBack
	case "tab", "shift+tab":
		return ActionFocusNext
	case "left", "h":
		return ActionLeft
	case "right", "l":
		return ActionRight
	case "up", "k":
		return ActionUp
	case "down", "j":
		return ActionDown
	case "1", "p":
		return ActionProjects
	case "2", "u":
		return ActionStudies
	case "r":
		return ActionRefresh
	case "y":
		return ActionConfirm
	case "enter", "o":
		return ActionOpen
	default:
		return ActionNone
	}
}

func HelpText() string {
	return "q quit | tab focus | arrows select | enter open | y confirm | esc back | r refresh"
}
