package sprint

import (
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

func TestResolveExecuteModelPrecedence(t *testing.T) {
	c := config.Defaults()
	c.Models.Default = "default/model"
	c.Models.Primary = "primary/model"
	c.Planning.PlanModel = "plan/model"
	c.Planning.ExecuteModel = "execute/model"

	cases := []struct {
		name     string
		override string
		mutate   func(*config.Config)
		want     ExecuteModelSelection
	}{
		{name: "override", override: "override/model", want: ExecuteModelSelection{Model: "override/model", Source: "command override"}},
		{name: "execute", want: ExecuteModelSelection{Model: "execute/model", Source: "planning.execute_model"}},
		{name: "planning fallback", mutate: func(c *config.Config) { c.Planning.ExecuteModel = "" }, want: ExecuteModelSelection{Model: "plan/model", Source: "planning.plan_model"}},
		{name: "primary fallback", mutate: func(c *config.Config) { c.Planning.ExecuteModel = ""; c.Planning.PlanModel = "" }, want: ExecuteModelSelection{Model: "primary/model", Source: "models.primary"}},
		{name: "default fallback", mutate: func(c *config.Config) { c.Planning.ExecuteModel = ""; c.Planning.PlanModel = ""; c.Models.Primary = "" }, want: ExecuteModelSelection{Model: "default/model", Source: "models.default"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := c
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			got, err := ResolveExecuteModel(cfg, tc.override)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveExecuteModelRejectsMissing(t *testing.T) {
	c := config.Defaults()
	c.Models.Default = ""
	c.Models.Primary = ""
	if _, err := ResolveExecuteModel(c, ""); err == nil {
		t.Fatalf("expected missing model error")
	}
}
