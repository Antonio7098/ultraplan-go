package sprint

import (
	"fmt"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

type ExecuteModelSelection struct {
	Model  string
	Source string
}

func ResolveExecuteModel(c config.Config, explicitOverride string) (ExecuteModelSelection, error) {
	candidates := []ExecuteModelSelection{
		{Model: explicitOverride, Source: "command override"},
		{Model: c.Planning.ExecuteModel, Source: "planning.execute_model"},
		{Model: c.Planning.PlanModel, Source: "planning.plan_model"},
		{Model: c.Models.Primary, Source: "models.primary"},
		{Model: c.Models.Default, Source: "models.default"},
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Model) == "" {
			continue
		}
		return candidate, nil
	}
	return ExecuteModelSelection{}, fmt.Errorf("execute model: no model configured")
}
