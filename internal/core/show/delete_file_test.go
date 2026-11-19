package show

import (
	"context"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

type deleteFileMock struct {
	db.Querier
	deleted []string
}

func (m *deleteFileMock) GetEpisodeFile(_ context.Context, id string) (db.EpisodeFile, error) {
	return db.EpisodeFile{ID: id, EpisodeID: "ep-1", SeriesID: "s-1", Path: "/tv/Show/Season 01/ep.mkv"}, nil
}
func (m *deleteFileMock) GetSeries(_ context.Context, id string) (db.Series, error) {
	return db.Series{ID: id, LibraryID: "lib"}, nil
}
func (m *deleteFileMock) GetLibrary(_ context.Context, id string) (db.Library, error) {
	return db.Library{ID: id, RootPath: "/tv"}, nil
}
func (m *deleteFileMock) DeleteEpisodeFile(_ context.Context, id string) error {
	m.deleted = append(m.deleted, id)
	return nil
}
func (m *deleteFileMock) GetEpisode(_ context.Context, id string) (db.Episode, error) {
	return db.Episode{ID: id}, nil
}
func (m *deleteFileMock) UpdateEpisode(_ context.Context, arg db.UpdateEpisodeParams) (db.Episode, error) {
	return db.Episode{ID: arg.ID}, nil
}

type fakeRecycler struct{ path, root string }

func (r *fakeRecycler) Delete(_ context.Context, path, root string) (string, error) {
	r.path, r.root = path, root
	return "/recycle/Show/Season 01/ep.mkv", nil
}

// "Delete from disk" goes through the recycle bin, with the library root
// so the file keeps its folders there.
func TestDeleteFile_FromDiskUsesRecycleBin(t *testing.T) {
	m := &deleteFileMock{}
	svc := NewService(m, nil, nil, nil, discardLogger())
	r := &fakeRecycler{}
	svc.SetRecycler(r)
	if err := svc.DeleteFile(context.Background(), "f-1", true); err != nil {
		t.Fatal(err)
	}
	if r.path != "/tv/Show/Season 01/ep.mkv" || r.root != "/tv" {
		t.Fatalf("recycler got path=%q root=%q", r.path, r.root)
	}
	if len(m.deleted) != 1 {
		t.Fatalf("record not deleted: %v", m.deleted)
	}

	// Keeping the file on disk never touches it.
	r2 := &fakeRecycler{}
	svc.SetRecycler(r2)
	if err := svc.DeleteFile(context.Background(), "f-1", false); err != nil {
		t.Fatal(err)
	}
	if r2.path != "" {
		t.Fatal("file recycled although delete_from_disk was false")
	}
}
