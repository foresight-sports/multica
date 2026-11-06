package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSupportReplyEscapesMentionsAndKeepsReceipt(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		received = map[string]any{}
		for k, v := range r.PostForm {
			if len(v) > 0 {
				received[k] = v[0]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"channel":"C1","ts":"1.2"}`))
	}))
	defer server.Close()
	c := SupportClient{APIURL: server.URL + "/"}
	ts, e := c.Reply(context.Background(), []byte(replierConfigJSON), "C1", "1.1", "receipt-id", "Finding <!channel> <@UOTHER>", "Multica APP-42", "UKODY")
	if e != nil || ts != "1.2" {
		t.Fatalf("%s %v", ts, e)
	}
	raw, _ := json.Marshal(received)
	text := received["text"].(string)
	if strings.Contains(text, "<!channel>") || strings.Contains(text, "<@UOTHER>") || !strings.Contains(text, "<@UKODY>") {
		t.Fatal(text)
	}
	if received["thread_ts"] != "1.1" || !strings.Contains(string(raw), "receipt-id") || !strings.Contains(string(raw), "APP-42") {
		t.Fatal(string(raw))
	}
}
