package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"gemfactory/internal/model"
)

type stubArtists struct {
	model.ArtistRepository
	byID map[int]*model.Artist
}

func (s *stubArtists) GetByID(_ context.Context, id int) (*model.Artist, error) {
	return s.byID[id], nil
}

func (s *stubArtists) GetByName(_ context.Context, name string) (*model.Artist, error) {
	for _, a := range s.byID {
		if strings.EqualFold(a.Name, name) {
			return a, nil
		}
	}
	return nil, nil
}

func (s *stubArtists) Update(_ context.Context, a *model.Artist) error {
	s.byID[a.ArtistID] = a
	return nil
}

func renameArtist(s *Server, id, name string) int {
	req := httptest.NewRequest("PATCH", "/api/artists/"+id, strings.NewReader(`{"name":"`+name+`"}`))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.updateArtist(rec, req)
	return rec.Code
}

func TestUpdateArtistRename(t *testing.T) {
	repo := &stubArtists{byID: map[int]*model.Artist{
		1: {ArtistID: 1, Name: "ive"},
		2: {ArtistID: 2, Name: "ITZY"},
	}}
	s := &Server{logger: zap.NewNop(), Deps: Deps{Artists: repo}}

	require.Equal(t, 409, renameArtist(s, "1", "itzy"), "name taken by another artist")
	require.Equal(t, "ive", repo.byID[1].Name)

	require.Equal(t, 200, renameArtist(s, "1", "IVE"), "case change of the same artist")
	require.Equal(t, "IVE", repo.byID[1].Name)
}
