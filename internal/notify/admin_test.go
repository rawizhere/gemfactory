package notify

import (
	"context"
	"testing"

	"gemfactory/internal/model"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubConfigRepo struct {
	values map[string]string
}

func (s *stubConfigRepo) Get(_ context.Context, key string) (*model.Config, error) {
	if v, ok := s.values[key]; ok {
		return &model.Config{Key: key, Value: v}, nil
	}
	return nil, nil
}

func (s *stubConfigRepo) GetAll(context.Context) ([]model.Config, error) { return nil, nil }
func (s *stubConfigRepo) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func (s *stubConfigRepo) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}
func (s *stubConfigRepo) Reset(context.Context) error { return nil }

func TestAdminNotifierSendWithoutChatID(t *testing.T) {
	n := NewAdminNotifier(&stubConfigRepo{values: map[string]string{}}, nil, zap.NewNop())
	require.NotPanics(t, func() { n.Send(context.Background(), "test") })
}

func TestAdminNotifierSendNil(t *testing.T) {
	var n *AdminNotifier
	require.NotPanics(t, func() { n.Send(context.Background(), "test") })
}
