package app

import (
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
)

func stagePublisher(cfg config.Config) gitpublish.Publisher {
	if cfg.Git.StageCompletion == string(gitpublish.ModeOff) {
		return nil
	}
	timeout, _ := time.ParseDuration(cfg.Git.PushTimeout)
	return gitpublish.New(gitpublish.Policy{
		Mode:        gitpublish.Mode(cfg.Git.StageCompletion),
		Remote:      cfg.Git.Remote,
		PushTimeout: timeout,
	})
}
