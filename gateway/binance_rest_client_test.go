package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBinanceRESTClientPlaceCancel(t *testing.T) {
	timeNowMillis = func() int64 { return 1234567890000 } // deterministic
	defer func() { timeNowMillis = func() int64 { return time.Now().UnixMilli() } }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if !strings.Contains(r.URL.RawQuery, "signature=") {
				t.Fatalf("missing signature")
			}
			io.WriteString(w, `{"orderId":"1001"}`)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(200)
			return
		}
		t.Fatalf("unexpected method %s", r.Method)
	}))
	defer ts.Close()

	cli := &BinanceRESTClient{
		BaseURL:    ts.URL,
		APIKey:     "key",
		Secret:     "secret",
		HTTPClient: ts.Client(),
	}
	id, err := cli.PlaceLimit("BTCUSDT", "BUY", "GTC", 100, 1, false, true, "cid")
	if err != nil {
		t.Fatalf("place err: %v", err)
	}
	if id != "1001" {
		t.Fatalf("unexpected order id %s", id)
	}
	if err := cli.CancelOrder("BTCUSDT", id); err != nil {
		t.Fatalf("cancel err: %v", err)
	}
}
