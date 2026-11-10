package settings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gemfactory/internal/model"
)

type countingRepo struct {
	values map[string]string
	loads  int
}

func (r *countingRepo) Get(context.Context, string) (*model.Config, error) {
	panic("reads must hit the cache")
}

func (r *countingRepo) GetAll(context.Context) ([]model.Config, error) {
	r.loads++
	out := make([]model.Config, 0, len(r.values))
	for k, v := range r.values {
		out = append(out, model.Config{Key: k, Value: v})
	}
	return out, nil
}

func (r *countingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestProviderResolveAndCache(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CLIP_PRESET", "slow")
	t.Setenv("CLIP_CRF", "")
	repo := &countingRepo{values: map[string]string{"TG_FILE_LIMIT_MB": " 100 ", "CLIP_MAX_DURATION_SECONDS": "bad"}}
	p := New(repo)

	v, src := p.Resolve(ctx, TGFileLimitMB)
	require.Equal(t, []string{"100", "db"}, []string{v, src})
	v, src = p.Resolve(ctx, ClipPreset)
	require.Equal(t, []string{"slow", "env"}, []string{v, src})
	v, src = p.Resolve(ctx, ClipCRF)
	require.Equal(t, []string{"20", "default"}, []string{v, src})
	require.Equal(t, 300, p.Int(ctx, ClipMaxDurationSeconds), "unparseable falls back to the default")
	require.True(t, p.Bool(ctx, GrokEnabled))
	require.Equal(t, 1, repo.loads)

	require.NoError(t, p.Set(ctx, ClipCRF.Name, "25"))
	require.Equal(t, 25, p.Int(ctx, ClipCRF))
	require.Equal(t, 2, repo.loads, "Set refreshes the cache")
}

func TestNilProviderUsesEnvAndDefaults(t *testing.T) {
	t.Setenv("GEMINI_MODEL", "")
	var p *Provider
	require.Equal(t, GeminiModel.Def, p.Value(context.Background(), GeminiModel))
}

func TestKeyCheck(t *testing.T) {
	v, err := ClipDeleteStatus.Check(" true ")
	require.NoError(t, err)
	require.Equal(t, "true", v)
	_, err = ClipDeleteStatus.Check("yes")
	require.Error(t, err)
	_, err = ClipCRF.Check("99")
	require.Error(t, err)
	v, err = ClipCRF.Check(" ")
	require.NoError(t, err, "empty clears and skips validation")
	require.Empty(t, v)
	require.Error(t, validateUserAgents("# only a comment\n"))
}
