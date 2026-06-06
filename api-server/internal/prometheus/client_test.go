package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQueryVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != `up{namespace="upm-clickhouse"}` {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{"pod":"clickhouse-0"},"value":[1,"1"]}]}}`))
	}))
	defer server.Close()

	samples, err := NewClient(server.URL, time.Second).Query(context.Background(), `up{namespace="upm-clickhouse"}`)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(samples) != 1 || samples[0].Metric["pod"] != "clickhouse-0" || samples[0].Value != 1 {
		t.Fatalf("unexpected samples: %#v", samples)
	}
}

func TestQueryRejectsPrometheusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"invalid query"}`))
	}))
	defer server.Close()

	if _, err := NewClient(server.URL, time.Second).Query(context.Background(), "bad"); err == nil {
		t.Fatal("expected Prometheus error")
	}
}
