package tmdbtv

import (
	"context"
	"net/http"
	"testing"
)

func TestGetSeries_ReadsExternalIDs(t *testing.T) {
	var gotAppend string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAppend = r.URL.Query().Get("append_to_response")
		_, _ = w.Write([]byte(`{"id":83867,"name":"Andor","first_air_date":"2022-09-21",
			"external_ids":{"imdb_id":"tt9253284","tvdb_id":393189}}`))
	})
	d, err := c.GetSeries(context.Background(), 83867)
	if err != nil {
		t.Fatal(err)
	}
	if gotAppend != "external_ids" {
		t.Errorf("append_to_response = %q, want external_ids (one request, not two)", gotAppend)
	}
	if d.IMDBID != "tt9253284" || d.TVDBID != 393189 {
		t.Errorf("ids = %q / %d", d.IMDBID, d.TVDBID)
	}
}

func TestGetExternalIDs(t *testing.T) {
	var gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":1,"imdb_id":null,"tvdb_id":81189}`))
	})
	ids, err := c.GetExternalIDs(context.Background(), 1396)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/tv/1396/external_ids" && gotPath != "/3/tv/1396/external_ids" {
		t.Errorf("path = %q", gotPath)
	}
	if ids.TVDBID != 81189 || ids.IMDBID != "" {
		t.Errorf("ids = %+v, want tvdb 81189 and no imdb (null)", ids)
	}
}
