package config

import "strings"

const redacted = "[REDACTED]"

type Redacted struct {
	Version    int               `json:"version"`
	Runtime    Runtime           `json:"runtime"`
	Models     Models            `json:"models"`
	Execution  Execution         `json:"execution"`
	Planning   Planning          `json:"planning"`
	QA         QA                `json:"qa"`
	Smoke      Smoke             `json:"smoke"`
	Git        Git               `json:"git"`
	RunControl RunControl        `json:"run_control"`
	Logging    Logging           `json:"logging"`
	Agentwrap  Agentwrap         `json:"agentwrap"`
	Sources    map[string]string `json:"sources,omitempty"`
}

func Redact(e Effective) Redacted {
	qa := e.Config.QA
	qa.Model = RedactValue("qa.model", qa.Model)
	qa.MapperModel = RedactValue("qa.mapper_model", qa.MapperModel)
	qa.InvestigatorModel = RedactValue("qa.investigator_model", qa.InvestigatorModel)
	qa.ChallengerModel = RedactValue("qa.challenger_model", qa.ChallengerModel)
	qa.ArbiterModel = RedactValue("qa.arbiter_model", qa.ArbiterModel)
	qa.ReconcilerModel = RedactValue("qa.reconciler_model", qa.ReconcilerModel)
	qa.EvaluatorModel = RedactValue("qa.evaluator_model", qa.EvaluatorModel)
	qa.RepairModel = RedactValue("qa.repair_model", qa.RepairModel)
	return Redacted{Version: e.Config.Version, Runtime: e.Config.Runtime, Models: redactModels(e.Config.Models), Execution: e.Config.Execution, Planning: redactPlanning(e.Config.Planning), QA: qa, Smoke: e.Config.Smoke, Git: e.Config.Git, RunControl: e.Config.RunControl, Logging: e.Config.Logging, Agentwrap: redactAgentwrap(e.Config.Agentwrap), Sources: e.Sources}
}

func Sensitive(key, value string) bool {
	s := strings.ToLower(key + " " + value)
	for _, marker := range []string{"secret", "token", "password", "apikey", "api_key", "api-key", "credential", "--key", "-key", " key="} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	for _, marker := range []string{"bearer ", "sk-", "ghp_", "github_pat_", "xoxb-", "aws_secret_access_key"} {
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

func redactPlanning(p Planning) Planning {
	p.RequirementsModel = RedactValue("planning.requirements_model", p.RequirementsModel)
	p.CodeContextModel = RedactValue("planning.code_context_model", p.CodeContextModel)
	p.CodeContextVariant = RedactValue("planning.code_context_variant", p.CodeContextVariant)
	p.SprintIndexModel = RedactValue("planning.sprint_index_model", p.SprintIndexModel)
	p.TechnicalHandbookModel = RedactValue("planning.technical_handbook_model", p.TechnicalHandbookModel)
	p.AreaReasoningModel = RedactValue("planning.area_reasoning_model", p.AreaReasoningModel)
	p.ProjectReasoningModel = RedactValue("planning.project_reasoning_model", p.ProjectReasoningModel)
	p.ReasoningModel = RedactValue("planning.reasoning_model", p.ReasoningModel)
	p.PlanModel = RedactValue("planning.plan_model", p.PlanModel)
	p.ExecuteModel = RedactValue("planning.execute_model", p.ExecuteModel)
	p.ExecuteVariant = RedactValue("planning.execute_variant", p.ExecuteVariant)
	p.ReviewModel = RedactValue("planning.review_model", p.ReviewModel)
	p.ReviewVariant = RedactValue("planning.review_variant", p.ReviewVariant)
	p.SmokeModel = RedactValue("planning.smoke_model", p.SmokeModel)
	p.SmokeVariant = RedactValue("planning.smoke_variant", p.SmokeVariant)
	return p
}

func redactAgentwrap(a Agentwrap) Agentwrap {
	a.Executable = RedactValue("agentwrap.executable", a.Executable)
	for i, value := range a.Env {
		a.Env[i] = RedactValue("agentwrap.env", value)
	}
	return a
}
