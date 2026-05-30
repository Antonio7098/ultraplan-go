package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"ultraplan-go/internal/platform/config"
)

type Logger struct {
	out    io.Writer
	format string
	level  string
}

func New(out io.Writer, format, level string) Logger {
	if out == nil {
		out = io.Discard
	}
	if format == "" {
		format = "text"
	}
	if level == "" {
		level = "info"
	}
	return Logger{out: out, format: format, level: level}
}

func (l Logger) Info(message string, fields map[string]string) error {
	return l.log("info", message, fields)
}

func (l Logger) Error(message string, fields map[string]string) error {
	return l.log("error", message, fields)
}

func (l Logger) log(level, message string, fields map[string]string) error {
	redacted := make(map[string]string, len(fields))
	for k, v := range fields {
		redacted[k] = config.RedactValue(k, v)
	}
	if l.format == "json" {
		payload := map[string]any{"level": level, "message": config.RedactValue("message", message)}
		for k, v := range redacted {
			payload[k] = v
		}
		return json.NewEncoder(l.out).Encode(payload)
	}
	keys := make([]string, 0, len(redacted))
	for k := range redacted {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if _, err := fmt.Fprintf(l.out, "level=%s message=%q", level, config.RedactValue("message", message)); err != nil {
		return err
	}
	for _, k := range keys {
		if _, err := fmt.Fprintf(l.out, " %s=%q", k, redacted[k]); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(l.out)
	return err
}
