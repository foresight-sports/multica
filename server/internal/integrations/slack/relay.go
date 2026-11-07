package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/slack-go/slack/slackevents"
)

const RelayMaxBytes = 1048576

// RelayCredential is loaded only from a server-mounted secret file, never from
// installation JSON or a task/browser API. Each key grants one installation.
type RelayCredential struct {
	InstallationID string `json:"installation_id"`
	WorkspaceID    string `json:"workspace_id"`
	AppID          string `json:"app_id"`
	TeamID         string `json:"team_id"`
	Secret         string `json:"secret"`
}

func LoadRelayCredentials() (map[string]RelayCredential, error) {
	path := os.Getenv("MULTICA_SLACK_RELAY_CONFIG_FILE")
	out := map[string]RelayCredential{}
	if path == "" {
		return out, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read Slack relay secret file")
	}
	var entries []RelayCredential
	if len(body) > 65536 || json.Unmarshal(body, &entries) != nil {
		return nil, errors.New("invalid Slack relay secret file")
	}
	for _, c := range entries {
		_, wsErr := util.ParseUUID(c.WorkspaceID)
		_, idErr := util.ParseUUID(c.InstallationID)
		if wsErr != nil || idErr != nil || len(c.Secret) < 32 || !relayAppID.MatchString(c.AppID) || !relayTeamID.MatchString(c.TeamID) {
			return nil, errors.New("invalid Slack relay credential binding")
		}
		if _, exists := out[c.InstallationID]; exists {
			return nil, errors.New("duplicate Slack relay credential binding")
		}
		out[c.InstallationID] = c
	}
	return out, nil
}

var relayAppID = regexp.MustCompile(`^A[A-Z0-9]{5,}$`)
var relayTeamID = regexp.MustCompile(`^T[A-Z0-9]{5,}$`)
var relayChannelID = regexp.MustCompile(`^[CG][A-Z0-9]{5,}$`)
var relayTimestamp = regexp.MustCompile(`^[0-9]{1,20}\.[0-9]{1,6}$`)
var relayNonce = regexp.MustCompile(`^[a-f0-9]{32}$`)

// VerifyRelaySignature authenticates the exact method, path and bytes. The
// handler also claims the nonce durably, across processes/restarts.
func VerifyRelaySignature(c RelayCredential, r *http.Request, body []byte, now time.Time) bool {
	stamp, nonce := r.Header.Get("X-Relay-Timestamp"), r.Header.Get("X-Relay-Nonce")
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || seconds < now.Unix()-300 || seconds > now.Unix()+300 || !relayNonce.MatchString(nonce) || r.URL.RawQuery != "" {
		return false
	}
	digest := sha256.Sum256(body)
	canonical := fmt.Sprintf("v1\n%s\n%s\n%s\n%s\n%x", stamp, nonce, r.Method, r.URL.EscapedPath(), digest)
	mac := hmac.New(sha256.New, []byte(c.Secret))
	mac.Write([]byte(canonical))
	signature, err := hex.DecodeString(r.Header.Get("X-Relay-Signature"))
	return err == nil && hmac.Equal(mac.Sum(nil), signature)
}

type RelayEvent struct {
	ChannelID string
	MessageTS string
	Message   channel.InboundMessage
	Eligible  bool
}

// ParseRelayEvent uses the same normalization and attachment URL allowlist as
// Socket Mode. Identity is checked before any content reaches the intake path.
func ParseRelayEvent(body []byte, binding RelayCredential, botUserID string) (RelayEvent, error) {
	var envelope struct {
		Type   string `json:"type"`
		AppID  string `json:"api_app_id"`
		TeamID string `json:"team_id"`
		Event  struct {
			Type        string            `json:"type"`
			Channel     string            `json:"channel"`
			ChannelType string            `json:"channel_type"`
			TS          string            `json:"ts"`
			ThreadTS    string            `json:"thread_ts"`
			Text        string            `json:"text"`
			Files       []json.RawMessage `json:"files"`
		} `json:"event"`
	}
	if len(body) > RelayMaxBytes || json.Unmarshal(body, &envelope) != nil {
		return RelayEvent{}, errors.New("invalid relay payload")
	}
	e := envelope.Event
	if envelope.Type != "event_callback" || envelope.AppID != binding.AppID || envelope.TeamID != binding.TeamID || (e.Type != "message" && e.Type != "app_mention") || !relayChannelID.MatchString(e.Channel) || !relayTimestamp.MatchString(e.TS) || (e.ThreadTS != "" && !relayTimestamp.MatchString(e.ThreadTS)) || e.ChannelType == "im" || e.ChannelType == "mpim" || len(e.Text) > 200000 || len(e.Files) > 100 {
		return RelayEvent{}, errors.New("invalid relay identity or message bounds")
	}
	parsed, err := slackevents.ParseEvent(body, slackevents.OptionNoVerifyToken())
	if err != nil {
		return RelayEvent{}, errors.New("invalid Slack event")
	}
	c := slackChannel{botUserID: botUserID}
	msg, eligible := c.normalizeEventsAPI(parsed, compileMentionRe(botUserID))
	return RelayEvent{ChannelID: e.Channel, MessageTS: e.TS, Message: msg, Eligible: eligible}, nil
}

// EnrichRelayMessage preserves the first event's text/identity and adds files
// from the duplicate message/app_mention variant. Files are keyed by Slack ID.
func EnrichRelayMessage(original, incoming channel.InboundMessage) (channel.InboundMessage, bool) {
	var a, b slackRawEvent
	if json.Unmarshal(original.Raw, &a) != nil || json.Unmarshal(incoming.Raw, &b) != nil {
		return original, false
	}
	seen := map[string]bool{}
	for _, f := range a.Files {
		seen[f.ID] = true
	}
	changed := false
	for _, f := range b.Files {
		if !seen[f.ID] {
			a.Files = append(a.Files, f)
			seen[f.ID] = true
			changed = true
		}
	}
	if changed {
		original.Raw, _ = json.Marshal(a)
	}
	return original, changed
}
