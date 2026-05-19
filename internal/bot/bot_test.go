package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// ── Fakes ─────────────────────────────────────────────────────────────────────

type fakeIPGetter struct {
	ip     string
	source string
	err    error
}

func (f *fakeIPGetter) GetPublicIP(_ context.Context) (string, string, error) {
	return f.ip, f.source, f.err
}

type fakeSender struct {
	channelID string
	sent      []*discordgo.MessageSend
}

func (f *fakeSender) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.channelID = channelID
	f.sent = append(f.sent, data)
	return &discordgo.Message{}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestBot(ip IPGetter) *Bot {
	return &Bot{
		ip:     ip,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func newMessage(channelID, msgID, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        msgID,
			ChannelID: channelID,
			Content:   content,
			Author:    &discordgo.User{ID: "user-id"},
		},
	}
}

// ── handlePiss ────────────────────────────────────────────────────────────────

func TestHandlePiss(t *testing.T) {
	t.Run("sends formatted reply to correct channel", func(t *testing.T) {
		sender := &fakeSender{}
		newTestBot(&fakeIPGetter{ip: "1.2.3.4", source: "https://api.ipify.org"}).
			handlePiss(sender, newMessage("chan-1", "msg-1", "!piss"))

		if len(sender.sent) != 1 {
			t.Fatalf("sent %d messages, want 1", len(sender.sent))
		}
		if sender.channelID != "chan-1" {
			t.Errorf("channel: got %q, want %q", sender.channelID, "chan-1")
		}
		if !strings.Contains(sender.sent[0].Content, "1.2.3.4") {
			t.Errorf("reply missing IP: %q", sender.sent[0].Content)
		}
	})

	t.Run("strips https scheme from display text", func(t *testing.T) {
		sender := &fakeSender{}
		newTestBot(&fakeIPGetter{ip: "1.2.3.4", source: "https://api.ipify.org"}).
			handlePiss(sender, newMessage("c", "m", "!piss"))

		content := sender.sent[0].Content
		if !strings.Contains(content, "[api.ipify.org]") {
			t.Errorf("display should strip https scheme: %q", content)
		}
		if !strings.Contains(content, "(https://api.ipify.org)") {
			t.Errorf("link target should keep full URL: %q", content)
		}
	})

	t.Run("strips http scheme from display text", func(t *testing.T) {
		sender := &fakeSender{}
		newTestBot(&fakeIPGetter{ip: "5.6.7.8", source: "http://checkip.amazonaws.com"}).
			handlePiss(sender, newMessage("c", "m", "!piss"))

		if !strings.Contains(sender.sent[0].Content, "[checkip.amazonaws.com]") {
			t.Errorf("display should strip http scheme: %q", sender.sent[0].Content)
		}
	})

	t.Run("reply references triggering message", func(t *testing.T) {
		sender := &fakeSender{}
		newTestBot(&fakeIPGetter{ip: "1.2.3.4", source: "https://api.ipify.org"}).
			handlePiss(sender, newMessage("c", "msg-42", "!piss"))

		ref := sender.sent[0].Reference
		if ref == nil || ref.MessageID != "msg-42" {
			t.Errorf("reply should reference triggering message ID")
		}
	})

	t.Run("reply does not ping author", func(t *testing.T) {
		sender := &fakeSender{}
		newTestBot(&fakeIPGetter{ip: "1.2.3.4", source: "https://api.ipify.org"}).
			handlePiss(sender, newMessage("c", "m", "!piss"))

		am := sender.sent[0].AllowedMentions
		if am == nil || am.RepliedUser {
			t.Error("reply should not ping the command author")
		}
	})

	t.Run("sends error reply when IP fetch fails", func(t *testing.T) {
		sender := &fakeSender{}
		newTestBot(&fakeIPGetter{err: errors.New("all sources failed")}).
			handlePiss(sender, newMessage("c", "m", "!piss"))

		if len(sender.sent) != 1 {
			t.Fatalf("sent %d messages, want 1", len(sender.sent))
		}
		if !strings.Contains(sender.sent[0].Content, "⚠️") {
			t.Errorf("error reply should contain warning emoji: %q", sender.sent[0].Content)
		}
	})
}
