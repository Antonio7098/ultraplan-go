package config

import "strings"

const redacted = "[REDACTED]"

type Redacted struct {
	Version   int               `json:"version"`
	Runtime   Runtime           `json:"runtime"`
	Models    Models            `json:"models"`
	Execution Execution         `json:"execution"`
	Logging   Logging           `json:"logging"`
	Agentwrap Agentwrap         `json:"agentwrap"`
	Sources   map[string]string `json:"sources,omitempty"`
}

func Redact(e Effective) Redacted {
	return Redacted{Version: e.Config.Version, Runtime: e.Config.Runtime, Models: redactModels(e.Config.Models), Execution: e.Config.Execution, Logging: e.Config.Logging, Agentwrap: redactAgentwrap(e.Config.Agentwrap), Sources: e.Sources}
}

func Sensitive(key, value string) bool {
	s := strings.ToLower(key + " " + value)
	for _, marker := range []string{"secret", "token", "password", "apikey", "api_key", "credential"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func RedactValue(key, value string) string {
	if Sensitive(key, value) {
		return redacted
	}
	return value
}

func redactModels(m Models) Models {
	return Models{Default: RedactValue("models.default", m.Default), Primary: RedactValue("models.primary", m.Primary), Backup: RedactValue("models.backup", m.Backup)}
}

func redactAgentwrap(a Agentwrap) Agentwrap {
	a.Executable = RedactValue("agentwrap.executable", a.Executable)
	for i, value := range a.Env {
		a.Env[i] = RedactValue("agentwrap.env", value)
	}
	return a
}
