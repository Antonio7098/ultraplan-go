package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Version   int       `json:"version"`
	Runtime   Runtime   `json:"runtime"`
	Models    Models    `json:"models"`
	Execution Execution `json:"execution"`
	Planning  Planning  `json:"planning"`
	Logging   Logging   `json:"logging"`
	Agentwrap Agentwrap `json:"agentwrap"`
}

type Runtime struct {
	Default string `json:"default"`
}
type Models struct {
	Default string `json:"default"`
	Primary string `json:"primary"`
	Backup  string `json:"backup"`
}
type Execution struct {
	DefaultVariant  string `json:"default_variant"`
	DefaultParallel int    `json:"default_parallel"`
	DefaultTimeout  string `json:"default_timeout"`
	DefaultRetries  int    `json:"default_retries"`
}
type Planning struct {
	RequirementsModel        string `json:"requirements_model"`
	RequirementsVariant      string `json:"requirements_variant"`
	SprintIndexModel         string `json:"sprint_index_model"`
	SprintIndexVariant       string `json:"sprint_index_variant"`
	TechnicalHandbookModel   string `json:"technical_handbook_model"`
	TechnicalHandbookVariant string `json:"technical_handbook_variant"`
	AreaReasoningModel       string `json:"area_reasoning_model"`
	AreaReasoningVariant     string `json:"area_reasoning_variant"`
	ReasoningModel           string `json:"reasoning_model"`
	ReasoningVariant         string `json:"reasoning_variant"`
	PlanModel                string `json:"plan_model"`
	PlanVariant              string `json:"plan_variant"`
	ExecuteModel             string `json:"execute_model"`
	ExecuteVariant           string `json:"execute_variant"`
}
type Logging struct {
	Format string `json:"format"`
	Level  string `json:"level"`
}
type Agentwrap struct {
	Executable                    string   `json:"executable"`
	ExtraArgs                     []string `json:"extra_args"`
	Env                           []string `json:"env"`
	StderrLimit                   int      `json:"stderr_limit"`
	RequiredHealth                []string `json:"required_health"`
	RequiredCapabilities          []string `json:"required_capabilities"`
	Sandbox                       string   `json:"sandbox"`
	PermissionMode                string   `json:"permission_mode"`
	PermissionDefault             string   `json:"permission_default"`
	PermissionUnsupportedBehavior string   `json:"permission_unsupported_behavior"`
}

type Effective struct {
	Config  Config
	Sources map[string]string
}

type CLIOverrides struct {
	LogFormat *string
	LogLevel  *string
	JSON      bool
}

type LoadOptions struct {
	WorkspaceRoot string
	Env           func(string) string
	CLI           CLIOverrides
}

type EnvOverride struct {
	Key   string
	Field string
}

func EnvOverrides() []EnvOverride {
	return []EnvOverride{
		{Key: "ULTRAPLAN_RUNTIME_DEFAULT", Field: "runtime.default"},
		{Key: "ULTRAPLAN_MODEL_DEFAULT", Field: "models.default"},
		{Key: "ULTRAPLAN_MODEL_PRIMARY", Field: "models.primary"},
		{Key: "ULTRAPLAN_MODEL_BACKUP", Field: "models.backup"},
		{Key: "ULTRAPLAN_DEFAULT_VARIANT", Field: "execution.default_variant"},
		{Key: "ULTRAPLAN_DEFAULT_PARALLEL", Field: "execution.default_parallel"},
		{Key: "ULTRAPLAN_DEFAULT_TIMEOUT", Field: "execution.default_timeout"},
		{Key: "ULTRAPLAN_DEFAULT_RETRIES", Field: "execution.default_retries"},
		{Key: "ULTRAPLAN_LOG_FORMAT", Field: "logging.format"},
		{Key: "ULTRAPLAN_LOG_LEVEL", Field: "logging.level"},
		{Key: "ULTRAPLAN_AGENTWRAP_EXECUTABLE", Field: "agentwrap.executable"},
		{Key: "ULTRAPLAN_AGENTWRAP_STDERR_LIMIT", Field: "agentwrap.stderr_limit"},
		{Key: "ULTRAPLAN_AGENTWRAP_SANDBOX", Field: "agentwrap.sandbox"},
		{Key: "ULTRAPLAN_AGENTWRAP_PERMISSION_MODE", Field: "agentwrap.permission_mode"},
	}
}

func Load(opts LoadOptions) (Effective, error) {
	e := Effective{Config: Defaults(), Sources: map[string]string{}}
	for _, field := range []string{"version", "runtime.default", "models.default", "models.primary", "models.backup", "execution.default_variant", "execution.default_parallel", "execution.default_timeout", "execution.default_retries", "planning.requirements_model", "planning.requirements_variant", "planning.sprint_index_model", "planning.sprint_index_variant", "planning.technical_handbook_model", "planning.technical_handbook_variant", "planning.area_reasoning_model", "planning.area_reasoning_variant", "planning.reasoning_model", "planning.reasoning_variant", "planning.plan_model", "planning.plan_variant", "planning.execute_model", "planning.execute_variant", "logging.format", "logging.level", "agentwrap.executable", "agentwrap.extra_args", "agentwrap.env", "agentwrap.stderr_limit", "agentwrap.required_health", "agentwrap.required_capabilities", "agentwrap.sandbox", "agentwrap.permission_mode", "agentwrap.permission_default", "agentwrap.permission_unsupported_behavior"} {
		e.Sources[field] = "default"
	}
	if opts.WorkspaceRoot != "" {
		if err := loadFile(filepath.Join(opts.WorkspaceRoot, "ultraplan.yml"), &e); err != nil {
			return e, err
		}
	}
	applyEnv(&e, opts.Env)
	if opts.CLI.LogFormat != nil {
		e.Config.Logging.Format = *opts.CLI.LogFormat
		e.Sources["logging.format"] = "cli"
	}
	if opts.CLI.LogLevel != nil {
		e.Config.Logging.Level = *opts.CLI.LogLevel
		e.Sources["logging.level"] = "cli"
	}
	if err := Validate(e.Config); err != nil {
		return e, err
	}
	return e, nil
}

func Defaults() Config {
	return Config{
		Version:   1,
		Runtime:   Runtime{Default: "opencode"},
		Models:    Models{Default: "provider/model", Primary: "provider/model", Backup: "provider/model"},
		Execution: Execution{DefaultVariant: "high", DefaultParallel: 3, DefaultTimeout: "30m", DefaultRetries: 3},
		Planning:  Planning{},
		Logging:   Logging{Format: "text", Level: "info"},
		Agentwrap: Agentwrap{Executable: "opencode", StderrLimit: 16 * 1024, RequiredHealth: []string{"runtime_available", "structured_output", "workdir"}, RequiredCapabilities: []string{"structured_events", "cancellation"}, Sandbox: "workspace_write", PermissionMode: "restricted", PermissionDefault: "ask"},
	}
}

func loadFile(path string, e *Effective) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read workspace config: %w", err)
	}
	defer file.Close()
	var section, listField string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, "-") {
			key := strings.TrimSuffix(line, ":")
			if leadingWhitespace(raw) > 0 && section != "" {
				field := section + "." + key
				if listConfigField(field) {
					clearListField(&e.Config, field)
					listField = field
					continue
				}
			}
			section = key
			listField = ""
			continue
		}
		if strings.HasPrefix(line, "- ") {
			item := strings.Trim(strings.TrimPrefix(line, "- "), `"`)
			switch listField {
			case "agentwrap.required_health":
				e.Config.Agentwrap.RequiredHealth = append(e.Config.Agentwrap.RequiredHealth, item)
				e.Sources[listField] = "workspace"
			case "agentwrap.required_capabilities":
				e.Config.Agentwrap.RequiredCapabilities = append(e.Config.Agentwrap.RequiredCapabilities, item)
				e.Sources[listField] = "workspace"
			case "agentwrap.extra_args":
				e.Config.Agentwrap.ExtraArgs = append(e.Config.Agentwrap.ExtraArgs, item)
				e.Sources[listField] = "workspace"
			case "agentwrap.env":
				e.Config.Agentwrap.Env = append(e.Config.Agentwrap.Env, item)
				e.Sources[listField] = "workspace"
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("parse workspace config: unsupported line %q", raw)
		}
		key, value := strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), `"`)
		field := key
		if section != "" {
			field = section + "." + key
		}
		if value == "" && listConfigField(field) {
			clearListField(&e.Config, field)
			listField = field
			continue
		}
		if err := setField(&e.Config, field, value); err != nil {
			return err
		}
		e.Sources[field] = "workspace"
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read workspace config: %w", err)
	}
	return nil
}

func leadingWhitespace(raw string) int {
	return len(raw) - len(strings.TrimLeft(raw, " \t"))
}

func applyEnv(e *Effective, env func(string) string) {
	if env == nil {
		return
	}
	for _, override := range EnvOverrides() {
		if value := env(override.Key); value != "" {
			_ = setField(&e.Config, override.Field, value)
			e.Sources[override.Field] = "env"
		}
	}
}

func setField(c *Config, field, value string) error {
	switch field {
	case "version":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("config version: must be an integer")
		}
		c.Version = n
	case "runtime.default":
		c.Runtime.Default = value
	case "models.default":
		c.Models.Default = value
	case "models.primary":
		c.Models.Primary = value
	case "models.backup":
		c.Models.Backup = value
	case "execution.default_variant":
		c.Execution.DefaultVariant = value
	case "execution.default_parallel":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("execution.default_parallel: must be an integer")
		}
		c.Execution.DefaultParallel = n
	case "execution.default_timeout":
		c.Execution.DefaultTimeout = value
	case "execution.default_retries":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("execution.default_retries: must be an integer")
		}
		c.Execution.DefaultRetries = n
	case "planning.requirements_model":
		c.Planning.RequirementsModel = value
	case "planning.requirements_variant":
		c.Planning.RequirementsVariant = value
	case "planning.sprint_index_model":
		c.Planning.SprintIndexModel = value
	case "planning.sprint_index_variant":
		c.Planning.SprintIndexVariant = value
	case "planning.technical_handbook_model":
		c.Planning.TechnicalHandbookModel = value
	case "planning.technical_handbook_variant":
		c.Planning.TechnicalHandbookVariant = value
	case "planning.area_reasoning_model":
		c.Planning.AreaReasoningModel = value
	case "planning.area_reasoning_variant":
		c.Planning.AreaReasoningVariant = value
	case "planning.reasoning_model":
		c.Planning.ReasoningModel = value
	case "planning.reasoning_variant":
		c.Planning.ReasoningVariant = value
	case "planning.plan_model":
		c.Planning.PlanModel = value
	case "planning.plan_variant":
		c.Planning.PlanVariant = value
	case "planning.execute_model":
		c.Planning.ExecuteModel = value
	case "planning.execute_variant":
		c.Planning.ExecuteVariant = value
	case "logging.format":
		c.Logging.Format = value
	case "logging.level":
		c.Logging.Level = value
	case "agentwrap.executable":
		c.Agentwrap.Executable = value
	case "agentwrap.stderr_limit":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("agentwrap.stderr_limit: must be an integer")
		}
		c.Agentwrap.StderrLimit = n
	case "agentwrap.sandbox":
		c.Agentwrap.Sandbox = value
	case "agentwrap.permission_mode":
		c.Agentwrap.PermissionMode = value
	case "agentwrap.permission_default":
		c.Agentwrap.PermissionDefault = value
	case "agentwrap.permission_unsupported_behavior":
		c.Agentwrap.PermissionUnsupportedBehavior = value
	default:
		return fmt.Errorf("unknown config field %q", field)
	}
	return nil
}

func Validate(c Config) error {
	checks := []struct{ field, value string }{
		{"runtime.default", c.Runtime.Default}, {"models.default", c.Models.Default}, {"models.primary", c.Models.Primary}, {"models.backup", c.Models.Backup}, {"execution.default_variant", c.Execution.DefaultVariant}, {"agentwrap.executable", c.Agentwrap.Executable},
	}
	if c.Version != 1 {
		return fmt.Errorf("version: expected schema version 1")
	}
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" {
			return fmt.Errorf("%s: value is required", check.field)
		}
	}
	if c.Runtime.Default != "opencode" {
		return fmt.Errorf("runtime.default: unsupported runtime %q", c.Runtime.Default)
	}
	if c.Execution.DefaultParallel <= 0 {
		return fmt.Errorf("execution.default_parallel: must be positive")
	}
	if c.Execution.DefaultRetries < 0 {
		return fmt.Errorf("execution.default_retries: must not be negative")
	}
	d, err := time.ParseDuration(c.Execution.DefaultTimeout)
	if err != nil || d <= 0 {
		return fmt.Errorf("execution.default_timeout: must be a positive duration")
	}
	if c.Logging.Format != "text" && c.Logging.Format != "json" {
		return fmt.Errorf("logging.format: must be text or json")
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level: must be debug, info, warn, or error")
	}
	if c.Agentwrap.StderrLimit <= 0 {
		return fmt.Errorf("agentwrap.stderr_limit: must be positive")
	}
	for _, h := range c.Agentwrap.RequiredHealth {
		if !knownHealth(h) {
			return fmt.Errorf("agentwrap.required_health: unsupported health check %q", h)
		}
	}
	for _, cap := range c.Agentwrap.RequiredCapabilities {
		if !knownCapability(cap) {
			return fmt.Errorf("agentwrap.required_capabilities: unsupported capability %q", cap)
		}
	}
	switch c.Agentwrap.PermissionDefault {
	case "", "allow", "deny", "ask":
	default:
		return fmt.Errorf("agentwrap.permission_default: must be allow, deny, or ask")
	}
	switch c.Agentwrap.PermissionUnsupportedBehavior {
	case "", "best_effort":
	default:
		return fmt.Errorf("agentwrap.permission_unsupported_behavior: must be best_effort or empty")
	}
	return nil
}

func listConfigField(field string) bool {
	switch field {
	case "agentwrap.required_health", "agentwrap.required_capabilities", "agentwrap.extra_args", "agentwrap.env":
		return true
	default:
		return false
	}
}

func clearListField(c *Config, field string) {
	switch field {
	case "agentwrap.required_health":
		c.Agentwrap.RequiredHealth = nil
	case "agentwrap.required_capabilities":
		c.Agentwrap.RequiredCapabilities = nil
	case "agentwrap.extra_args":
		c.Agentwrap.ExtraArgs = nil
	case "agentwrap.env":
		c.Agentwrap.Env = nil
	}
}

func knownHealth(value string) bool {
	switch value {
	case "runtime_available", "structured_output", "workdir", "config", "provider", "model", "authentication", "runtime_paths":
		return true
	default:
		return false
	}
}

func knownCapability(value string) bool {
	switch value {
	case "sessions", "session_continue", "session_fork", "session_replace", "session_release", "structured_events", "raw_payloads", "cancellation", "artifacts", "permissions", "usage", "validation_events":
		return true
	default:
		return false
	}
}
