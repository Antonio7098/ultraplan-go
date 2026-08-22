package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Antonio7098/agentwrap"
)

// ErrModelsUnavailable reports that the configured runtime cannot enumerate
// models.
var ErrModelsUnavailable = errors.New("runtime does not support model listing")

// Model describes one model available through the configured runtime.
type Model struct {
	Provider string
	ID       string
}

// ListModels enumerates models known to the runtime adapter, optionally
// filtered by provider. The listing is read-only and never starts agent work.
func (a Adapter) ListModels(ctx context.Context, provider string) ([]Model, error) {
	if a.runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	lister, ok := a.runtime.(agentwrap.ModelLister)
	if !ok {
		return nil, ErrModelsUnavailable
	}
	infos, err := lister.ListModels(ctx, agentwrap.ModelsRequest{Provider: agentwrap.ProviderID(provider)})
	if err != nil {
		return nil, mapError(err)
	}
	models := make([]Model, 0, len(infos))
	for _, info := range infos {
		models = append(models, Model{Provider: string(info.Provider), ID: string(info.ID)})
	}
	return models, nil
}

// MaxModelListing bounds how many models a selection surface may retain.
const MaxModelListing = 500
