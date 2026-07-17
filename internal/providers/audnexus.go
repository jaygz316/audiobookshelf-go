package providers

import (
	"context"
)

// AudnexusProvider searches Audnexus API.
type AudnexusProvider struct{}

func (p *AudnexusProvider) Name() string {
	return "audnexus"
}

func (p *AudnexusProvider) SearchPodcasts(ctx context.Context, query string) ([]*MetadataResult, error) {
	return nil, nil
}
