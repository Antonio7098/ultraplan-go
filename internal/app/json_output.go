package app

import (
	"encoding/json"
	"io"
	"time"
)

const jsonEnvelopeSchemaVersion = 1

type jsonEnvelope struct {
	SchemaVersion int       `json:"schema_version"`
	Command       string    `json:"command"`
	Workspace     string    `json:"workspace,omitempty"`
	Status        string    `json:"status"`
	GeneratedAt   time.Time `json:"generated_at"`
	Result        any       `json:"result"`
}

func writeJSON(w io.Writer, command, workspacePath, status string, result any) error {
	payload := jsonEnvelope{
		SchemaVersion: jsonEnvelopeSchemaVersion,
		Command:       command,
		Workspace:     workspacePath,
		Status:        status,
		GeneratedAt:   timeNow(),
		Result:        result,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
