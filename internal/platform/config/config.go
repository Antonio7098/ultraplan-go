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
type Logging struct {
	Format string `json:"format"`
	Level  string `json:"level"`
}
type Agentwrap struct {
	Executable     string   `json:"executable"`
	RequiredHealth []string `json:"required_health"`
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

func Load(opts LoadOptions) (Effective, error) {
	e := Effective{Config: Defaults(), Sources: map[string]string{}}
	for _, field := range []string{"version", "runtime.default", "models.default", "models.primary", "models.backup", "execution.default_variant", "execution.default_parallel", "execution.default_timeout", "execution.default_retries", "logging.format", "logging.level", "agentwrap.executable", "agentwrap.required_health"} {
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
		Logging:   Logging{Format: "text", Level: "info"},
		Agentwrap: Agentwrap{Executable: "opencode", RequiredHealth: []string{"runtime_available", "structured_output", "workdir"}},
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
			section = strings.TrimSuffix(line, ":")
			listField = ""
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if listField == "agentwrap.required_health" {
				e.Config.Agentwrap.RequiredHealth = append(e.Config.Agentwrap.RequiredHealth, strings.Trim(strings.TrimPrefix(line, "- "), `"`))
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
		if value == "" && field == "agentwrap.required_health" {
			e.Config.Agentwrap.RequiredHealth = nil
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

func applyEnv(e *Effective, env func(string) string) {
	if env == nil {
		return
	}
	for key, field := range map[string]string{
		"ULTRAPLAN_RUNTIME_DEFAULT":      "runtime.default",
		"ULTRAPLAN_MODEL_DEFAULT":        "models.default",
		"ULTRAPLAN_MODEL_PRIMARY":        "models.primary",
		"ULTRAPLAN_MODEL_BACKUP":         "models.backup",
		"ULTRAPLAN_DEFAULT_VARIANT":      "execution.default_variant",
		"ULTRAPLAN_DEFAULT_PARALLEL":     "execution.default_parallel",
		"ULTRAPLAN_DEFAULT_TIMEOUT":      "execution.default_timeout",
		"ULTRAPLAN_DEFAULT_RETRIES":      "execution.default_retries",
		"ULTRAPLAN_LOG_FORMAT":           "logging.format",
		"ULTRAPLAN_LOG_LEVEL":            "logging.level",
		"ULTRAPLAN_AGENTWRAP_EXECUTABLE": "agentwrap.executable",
	} {
		if value := env(key); value != "" {
			_ = setField(&e.Config, field, value)
			e.Sources[field] = "env"
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
	case "logging.format":
		c.Logging.Format = value
	case "logging.level":
		c.Logging.Level = value
	case "agentwrap.executable":
		c.Agentwrap.Executable = value
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
	for _, h := range c.Agentwrap.RequiredHealth {
		switch h {
		case "runtime_available", "structured_output", "workdir":
		default:
			return fmt.Errorf("agentwrap.required_health: unsupported health check %q", h)
		}
	}
	return nil
}
