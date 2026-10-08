// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package csm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// newSearchTestClient serves searchResponse for every /incidents/search and records each request body.
func newSearchTestClient(t *testing.T, searchResponse string) (*Client, func() []searchIncidentsRequest) {
	t.Helper()
	var mu sync.Mutex
	var searches []searchIncidentsRequest
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = io.WriteString(w, `{"access_token":"t","token_type":"bearer","expires_in":3600}`)
		case "/incidents/search":
			var req searchIncidentsRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			searches = append(searches, req)
			mu.Unlock()
			_, _ = io.WriteString(w, searchResponse)
		default:
			http.NotFound(w, r)
		}
	}))
	// The client only speaks HTTPS; trust the test server's certificate for this run.
	saved := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() {
		srv.Client().CloseIdleConnections()
		http.DefaultTransport = saved
		srv.Close()
	})
	client := NewClient(Config{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "c", ClientSecret: "s"})
	return client, func() []searchIncidentsRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]searchIncidentsRequest(nil), searches...)
	}
}

const testTag = "[fp:eaf2da8f3139:1791462025881]"

// A tag with no incident is a miss, found in a single exact correlationId search and never a free-text one.
func TestSearchIncidentByCorrelationID_NoMatchIsOneExactSearch(t *testing.T) {
	client, searches := newSearchTestClient(t, `{"incidents":[],"total":0}`)

	_, _, found, err := client.SearchIncidentByCorrelationID(context.Background(), testTag)
	if err != nil || found {
		t.Fatalf("found=%v err=%v; want a clean miss", found, err)
	}
	got := searches()
	if len(got) != 1 {
		t.Fatalf("made %d searches, want 1 (no free-text fallback)", len(got))
	}
	if got[0].Filters.CorrelationID != testTag || got[0].Filters.Number != "" {
		t.Errorf("filters = %+v, want correlationId only", got[0].Filters)
	}
}

func TestSearchIncidentByCorrelationID_SingleMatchIsReused(t *testing.T) {
	client, _ := newSearchTestClient(t, `{"incidents":[{"id":"inc-1","number":"INC0100206","state":"NEW"}],"total":1}`)

	id, number, found, err := client.SearchIncidentByCorrelationID(context.Background(), testTag)
	if err != nil || !found || id != "inc-1" || number != "INC0100206" {
		t.Fatalf("got id=%q number=%q found=%v err=%v; want inc-1/INC0100206", id, number, found, err)
	}
}

// A backend that drops the correlationId filter returns unrelated incidents; reusing the first one merged alerts into the wrong incident.
func TestSearchIncidentByCorrelationID_SeveralMatchesAreAnError(t *testing.T) {
	cases := map[string]string{
		"two rows":        `{"incidents":[{"id":"a","number":"INC0040603"},{"id":"b","number":"INC0054727"}],"total":2}`,
		"one row of many": `{"incidents":[{"id":"a","number":"INC0040603"}],"total":90000}`,
	}
	for name, resp := range cases {
		t.Run(name, func(t *testing.T) {
			client, _ := newSearchTestClient(t, resp)

			_, number, found, err := client.SearchIncidentByCorrelationID(context.Background(), testTag)
			if !errors.Is(err, ErrCorrelationFilterIgnored) || found {
				t.Fatalf("number=%q found=%v err=%v; want ErrCorrelationFilterIgnored, not a reused incident", number, found, err)
			}
		})
	}
}
