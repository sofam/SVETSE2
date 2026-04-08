package main

import (
	"log"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var discordMentionRe = regexp.MustCompile(`<@!?\d+>`)

func cleanDiscordText(text string) string {
	text = discordMentionRe.ReplaceAllString(text, "")
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func runDiscord(cfg Config, learnCh chan<- LearnRequest, replyCh chan<- ReplyRequest, helpCh chan<- HelpRequest) {
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		log.Fatalf("Discord session error: %v", err)
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	allowedChannels := make(map[string]bool)
	for _, ch := range cfg.DiscordChannels {
		allowedChannels[strings.TrimPrefix(ch, "#")] = true
	}

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}
		if m.Author.Bot {
			return
		}

		isMention := false
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				isMention = true
				break
			}
		}

		if !isMention {
			cleaned := cleanDiscordText(m.Content)
			if cleaned != "" {
				learnCh <- LearnRequest{Text: cleaned}
			}
			return
		}

		if len(allowedChannels) > 0 {
			ch, err := s.Channel(m.ChannelID)
			if err != nil || (!allowedChannels[m.ChannelID] && !allowedChannels[ch.Name]) {
				return
			}
		}

		text := cleanDiscordText(m.Content)
		text, overrides, isHelp := parseOverrides(text)

		if isHelp {
			rc := make(chan string, 1)
			helpCh <- HelpRequest{ReplyCh: rc}
			reply := <-rc
			s.ChannelMessageSend(m.ChannelID, reply)
			return
		}

		if strings.TrimSpace(text) == "" {
			return
		}

		rc := make(chan string, 1)
		replyCh <- ReplyRequest{Text: text, Overrides: overrides, ReplyCh: rc}
		reply := <-rc
		s.ChannelMessageSend(m.ChannelID, reply)
	})

	if err := dg.Open(); err != nil {
		log.Fatalf("Discord connection error: %v", err)
	}
	log.Println("Discord adapter connected")

	select {}
}
