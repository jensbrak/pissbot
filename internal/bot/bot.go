// Package bot wraps a discordgo session and handles the !piss command.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// IPGetter is the interface required by Bot to resolve the public IP.
// It is satisfied by *ipservice.Service.
type IPGetter interface {
	GetPublicIP(ctx context.Context) (ip, source string, err error)
}

// Bot manages the Discord connection and message handling.
type Bot struct {
	session *discordgo.Session
	ip      IPGetter
	logger  *slog.Logger
}

// New creates a Bot but does not connect yet — call Open to establish the
// WebSocket connection to the Discord gateway.
func New(token string, ip IPGetter, logger *slog.Logger) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// --- Intents ---
	// IntentsGuildMessages   : receive message events in servers.
	// IntentsDirectMessages  : receive message events in DMs.
	// IntentMessageContent   : read the actual message text (privileged intent —
	//                          must be enabled in the Discord Developer Portal
	//                          under Bot → Privileged Gateway Intents).
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	// ShouldReconnectOnError defaults to true in discordgo; the library handles
	// transient gateway disconnections and resumes the session automatically.
	// No additional reconnection logic is required here.

	b := &Bot{session: session, ip: ip, logger: logger}
	session.AddHandler(b.onReady)
	session.AddHandler(b.onMessage)
	return b, nil
}

// Open establishes the WebSocket connection to the Discord gateway.
func (b *Bot) Open() error {
	return b.session.Open()
}

// Close gracefully disconnects from Discord.
func (b *Bot) Close() error {
	return b.session.Close()
}

// onReady is called once the gateway handshake completes.
func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.logger.Info("connected to Discord",
		"user", r.User.String(),
		"guilds", len(r.Guilds),
		"session_id", r.SessionID,
	)
}

// onMessage is invoked for every incoming message. discordgo calls handlers
// from a dedicated goroutine per event type, so blocking here would stall all
// future message events. We spawn a goroutine for the slow work (HTTP + send)
// and return immediately.
func (b *Bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Skip own messages and other bots.
	if m.Author == nil || m.Author.Bot || m.Author.ID == s.State.User.ID {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(m.Content), "!piss") {
		return
	}

	go b.handlePiss(s, m)
}

func (b *Bot) handlePiss(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Give the whole operation (IP fetch + Discord send) 15 seconds.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	b.logger.Info("!piss received",
		"user", m.Author.String(),
		"channel", m.ChannelID,
		"guild", m.GuildID,
	)

	ip, source, err := b.ip.GetPublicIP(ctx)

	var content string
	if err != nil {
		b.logger.Error("public IP resolution failed", "error", err)
		content = "⚠️ Could not determine the public IP — all sources failed. Try again shortly."
	} else {
		b.logger.Info("serving public IP", "ip", ip, "source", source)
		// Strip the scheme so the display text reads cleanly, e.g. "api.ipify.org".
		// The full URL is kept as the link target so it remains clickable.
		display := strings.TrimPrefix(strings.TrimPrefix(source, "https://"), "http://")
		content = fmt.Sprintf("🌐 [%s](%s) says: `%s`", display, source, ip)
	}

	// Reply to the triggering message without pinging the author.
	msg := &discordgo.MessageSend{
		Content: content,
		Reference: &discordgo.MessageReference{
			MessageID: m.ID,
		},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{}, // suppress @mention parsing
			RepliedUser: false,                            // do not ping the command author
		},
	}
	if _, sendErr := s.ChannelMessageSendComplex(m.ChannelID, msg); sendErr != nil {
		b.logger.Error("failed to send reply", "error", sendErr)
	}
}
