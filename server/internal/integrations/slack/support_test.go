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
	ts, e := c.Reply(context.Background(), []byte(replierConfigJSON), "C1", "1.1", "receipt-id", "## Finding\n**Retry the connection**\n- Keep the device on. <!channel> <@UOTHER>", "Multica APP-42", "UKODY")
	if e != nil || ts != "1.2" {
		t.Fatalf("%s %v", ts, e)
	}
	raw, _ := json.Marshal(received)
	text := received["text"].(string)
	if strings.Contains(text, "<!channel>") || strings.Contains(text, "<@UOTHER>") || !strings.Contains(text, "<@UKODY>") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "*Finding*") || !strings.Contains(text, "*Retry the connection*") || strings.Contains(text, "**") || strings.Contains(text, "##") {
		t.Fatal("support Markdown was not converted: " + text)
	}
	if received["thread_ts"] != "1.1" || !strings.Contains(string(raw), "receipt-id") || !strings.Contains(string(raw), "APP-42") {
		t.Fatal(string(raw))
	}
}

func TestSupportDirectoryListsBotMembershipsInsteadOfWholeWorkspace(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		r.ParseForm()
		switch r.URL.Path {
		case "/users.conversations":
			pages++
			if r.Form.Get("user") != "" {
				t.Error("directory must use authenticated bot identity")
			}
			if pages == 1 {
				w.Write([]byte(`{"ok":true,"channels":[{"id":"CONE","name":"support"}],"response_metadata":{"next_cursor":"next"}}`))
			} else {
				if r.Form.Get("cursor") != "next" {
					t.Error("lost pagination cursor")
				}
				w.Write([]byte(`{"ok":true,"channels":[{"id":"CTWO","name":"other"}],"response_metadata":{"next_cursor":""}}`))
			}
		case "/users.list":
			w.Write([]byte(`{"ok":true,"members":[{"id":"UKODY","real_name":"Kody Peterson"},{"id":"UBOT","is_bot":true},{"id":"UGONE","deleted":true}]}`))
		default:
			t.Errorf("unexpected directory endpoint %s", r.URL.Path)
			w.Write([]byte(`{"ok":false,"error":"unexpected_endpoint"}`))
		}
	}))
	defer server.Close()
	c := SupportClient{APIURL: server.URL + "/"}
	out, err := c.Directory(context.Background(), []byte(replierConfigJSON))
	if err != nil {
		t.Fatal(err)
	}
	if pages != 2 || len(out.Channels) != 2 || len(out.People) != 1 || out.People[0].ID != "UKODY" {
		t.Fatalf("wrong directory %+v, pages %d", out, pages)
	}
}
