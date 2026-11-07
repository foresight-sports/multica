package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/slack-go/slack"
	"strings"
)

// SupportClient uses only the installation's encrypted bot credentials. Agent
// tasks never receive Slack tokens or a general-purpose posting capability.
type SupportClient struct {
	Decrypt Decrypter
	APIURL  string
}

// VerifyIdentity is read-only: it never opens a Socket Mode connection.
func (s *SupportClient) VerifyIdentity(ctx context.Context, config json.RawMessage) error {
	c, err := s.client(config)
	if err != nil {
		return fmt.Errorf("Slack bot credentials unavailable")
	}
	auth, err := c.AuthTestContext(ctx)
	info := DecodePublicConfig(config)
	if err != nil || auth.TeamID != info.TeamID || auth.UserID != info.BotUserID || auth.BotID == "" {
		return fmt.Errorf("Slack auth.test does not match the stored team and bot identity")
	}
	bot, err := c.GetBotInfoContext(ctx, slack.GetBotInfoParameters{Bot: auth.BotID})
	if err != nil || bot == nil || bot.AppID != info.AppID || bot.Deleted {
		return fmt.Errorf("Slack bots.info does not match the stored app identity")
	}
	return nil
}

func (s *SupportClient) VerifyChannel(ctx context.Context, config json.RawMessage, id string) (string, bool, error) {
	c, err := s.client(config)
	if err != nil {
		return "", false, fmt.Errorf("Slack bot credentials unavailable")
	}
	ch, err := c.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{ChannelID: id})
	if err != nil || ch == nil || ch.ID != id || !ch.IsMember || ch.IsArchived || ch.IsIM || ch.IsMpIM {
		return "", false, fmt.Errorf("Slack channel unavailable; verify bot membership and read scopes")
	}
	return ch.Name, ch.IsPrivate, nil
}

func (s *SupportClient) client(raw json.RawMessage) (*slack.Client, error) {
	c, e := decodeCredentials(raw, s.Decrypt)
	if e != nil {
		return nil, e
	}
	if s.APIURL != "" {
		return slack.New(c.BotToken, slack.OptionAPIURL(s.APIURL)), nil
	}
	return slack.New(c.BotToken), nil
}
func (s *SupportClient) Validate(ctx context.Context, raw json.RawMessage, channelID, userID string) (string, error) {
	c, e := s.client(raw)
	if e != nil {
		return "", e
	}
	ch, e := c.GetConversationInfoContext(ctx, &slack.GetConversationInfoInput{ChannelID: channelID})
	if e != nil {
		return "", fmt.Errorf("cannot read channel; check bot membership and channel read scopes")
	}
	if !ch.IsMember || ch.IsArchived || ch.IsIM || ch.IsMpIM {
		return "", fmt.Errorf("choose an active channel the bot has joined")
	}
	u, e := c.GetUserInfoContext(ctx, userID)
	if e != nil || u == nil || u.Deleted || u.IsBot {
		return "", fmt.Errorf("choose an active Slack person for escalation")
	}
	return ch.Name, nil
}
func (s *SupportClient) Reaction(ctx context.Context, raw json.RawMessage, ch, ts string, active bool) error {
	c, e := s.client(raw)
	if e != nil {
		return e
	}
	item := slack.ItemRef{Channel: ch, Timestamp: ts}
	if active {
		e = c.AddReactionContext(ctx, "robot_face", item)
	} else {
		e = c.RemoveReactionContext(ctx, "robot_face", item)
	}
	if e != nil && (e.Error() == "already_reacted" || e.Error() == "no_reaction") {
		return nil
	}
	return e
}
func (s *SupportClient) Reply(ctx context.Context, raw json.RawMessage, ch, thread, id, body, footer, mention string) (string, error) {
	c, e := s.client(raw)
	if e != nil {
		return "", e
	}
	// User/agent content cannot introduce mentions or Slack link entities.
	text := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(body)
	if mention != "" {
		text = "<@" + mention + "> " + text
	}
	blocks := []slack.Block{slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", text, false, false), nil, nil), slack.NewContextBlock("", slack.NewTextBlockObject("plain_text", footer, false, false))}
	_, ts, e := c.PostMessageContext(ctx, ch, slack.MsgOptionText(text+"\n"+footer, false), slack.MsgOptionBlocks(blocks...), slack.MsgOptionTS(thread), slack.MsgOptionDisableLinkUnfurl(), slack.MsgOptionDisableMediaUnfurl(), slack.MsgOptionMetadata(slack.SlackMetadata{EventType: "multica_support_reply", EventPayload: map[string]interface{}{"reply_id": id}}))
	return ts, e
}
func SupportSource(msg channel.InboundMessage) (appID, teamID string) {
	var raw slackRawEvent
	if json.Unmarshal(msg.Raw, &raw) == nil {
		return raw.APIAppID, raw.TeamID
	}
	return "", ""
}
func (s *SupportClient) File(ctx context.Context, config json.RawMessage, raw json.RawMessage, id string) ([]byte, string, error) {
	var event slackRawEvent
	if json.Unmarshal(raw, &event) != nil {
		return nil, "", fmt.Errorf("invalid Slack file receipt")
	}
	creds, e := decodeCredentials(config, s.Decrypt)
	if e != nil {
		return nil, "", e
	}
	for _, f := range event.Files {
		if f.ID == id {
			r := NewMediaResolver(s.Decrypt, nil, nil, nil).(*slackMediaResolver)
			return r.download(ctx, f.DownloadURL, creds.BotToken)
		}
	}
	return nil, "", fmt.Errorf("file not found on this support thread")
}

type SupportChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type SupportDirectory struct {
	Channels []SupportChoice `json:"channels"`
	People   []SupportChoice `json:"people"`
}

func (s *SupportClient) Directory(ctx context.Context, raw json.RawMessage) (SupportDirectory, error) {
	out := SupportDirectory{Channels: []SupportChoice{}, People: []SupportChoice{}}
	c, e := s.client(raw)
	if e != nil {
		return out, e
	}
	cursor := ""
	for page := 0; page < 10; page++ {
		chs, next, e := c.GetConversationsForUserContext(ctx, &slack.GetConversationsForUserParameters{Cursor: cursor, Limit: 200, ExcludeArchived: true, Types: []string{"public_channel", "private_channel"}})
		if e != nil {
			return out, fmt.Errorf("cannot load joined Slack channels; check channels:read and groups:read")
		}
		// users.conversations already restricts results to the bot's memberships;
		// its channel objects need not include conversations.list's is_member flag.
		for _, ch := range chs {
			out.Channels = append(out.Channels, SupportChoice{ch.ID, "#" + ch.Name})
		}
		if next == "" {
			break
		}
		cursor = next
		if page == 9 {
			return out, fmt.Errorf("joined Slack channel directory exceeded the pagination limit")
		}
	}
	users, e := c.GetUsersContext(ctx)
	if e != nil {
		return out, fmt.Errorf("cannot load Slack people; check users:read")
	}
	for _, u := range users {
		if !u.Deleted && !u.IsBot && u.ID != "USLACKBOT" {
			name := u.RealName
			if name == "" {
				name = u.Name
			}
			out.People = append(out.People, SupportChoice{u.ID, name})
		}
	}
	return out, nil
}

// A timed-out POST may have succeeded. Reconcile its unique metadata first;
// absence is not authority to retry an ambiguous external side effect.
func (s *SupportClient) FindReply(ctx context.Context, raw json.RawMessage, ch, root, id string) (string, error) {
	c, e := s.client(raw)
	if e != nil {
		return "", e
	}
	cursor := ""
	for page := 0; page < 10; page++ {
		msgs, more, next, e := c.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{ChannelID: ch, Timestamp: root, Cursor: cursor, Limit: 100, IncludeAllMetadata: true})
		if e != nil {
			return "", fmt.Errorf("could not reconcile Slack delivery")
		}
		for _, m := range msgs {
			if m.Metadata.EventType == "multica_support_reply" && m.Metadata.EventPayload["reply_id"] == id {
				return m.Timestamp, nil
			}
		}
		if !more {
			return "", nil
		}
		cursor = next
	}
	return "", fmt.Errorf("thread too long to reconcile delivery")
}
