package main

import (
	"log"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

var slackMentionRe = regexp.MustCompile(`<@[A-Z0-9]+>`)

func cleanSlackText(text string) string {
	text = regexp.MustCompile(`<#[A-Z0-9]+\|([^>]+)>`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`<(https?://[^|>]+)\|([^>]+)>`).ReplaceAllString(text, "$2")
	text = regexp.MustCompile(`<(https?://[^>]+)>`).ReplaceAllString(text, "$1")
	text = slackMentionRe.ReplaceAllString(text, "")
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}

func runSlack(cfg Config, learnCh chan<- LearnRequest, replyCh chan<- ReplyRequest, helpCh chan<- HelpRequest, trainCh chan<- TrainRequest) {
	api := slack.New(cfg.SlackToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	client := socketmode.New(api)

	authResp, err := api.AuthTest()
	if err != nil {
		log.Fatalf("Slack auth failed: %v", err)
	}
	botUserID := authResp.UserID
	mentionTag := "<@" + botUserID + ">"
	log.Printf("Slack bot user ID: %s", botUserID)

	allowedChannels := make(map[string]bool)
	for _, ch := range cfg.SlackChannels {
		allowedChannels[strings.TrimPrefix(ch, "#")] = true
	}

	go func() {
		for evt := range client.Events {
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				client.Ack(*evt.Request)

				switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					if ev.BotID != "" || ev.SubType != "" {
						continue
					}

					isMention := strings.Contains(ev.Text, mentionTag)

					if !isMention {
						cleaned := cleanSlackText(ev.Text)
						if cleaned != "" {
							learnCh <- LearnRequest{Text: cleaned}
						}
						continue
					}

					// Check channel allowlist
					if len(allowedChannels) > 0 && !allowedChannels[ev.Channel] {
						info, err := api.GetConversationInfo(&slack.GetConversationInfoInput{
							ChannelID: ev.Channel,
						})
						if err != nil || !allowedChannels[info.Name] {
							continue
						}
					}

					text := cleanSlackText(ev.Text)
					parsed := parseOverrides(text)

					if parsed.Help {
						rc := make(chan string, 1)
						helpCh <- HelpRequest{ReplyCh: rc}
						reply := <-rc
						api.PostMessage(ev.Channel, slack.MsgOptionText(reply, false))
						continue
					}

					if parsed.TrainURL != "" {
						rc := make(chan string, 1)
						trainCh <- TrainRequest{URL: parsed.TrainURL, ReplyCh: rc}
						reply := <-rc
						api.PostMessage(ev.Channel, slack.MsgOptionText(reply, false))
						continue
					}

					if strings.TrimSpace(parsed.Text) == "" {
						continue
					}

					rc := make(chan string, 1)
					replyCh <- ReplyRequest{Text: parsed.Text, Overrides: parsed.Overrides, ReplyCh: rc}
					reply := <-rc
					api.PostMessage(ev.Channel, slack.MsgOptionText(reply, false))
				}

			case socketmode.EventTypeConnecting:
				log.Println("Slack: connecting...")
			case socketmode.EventTypeConnected:
				log.Println("Slack: connected")
			case socketmode.EventTypeConnectionError:
				log.Println("Slack: connection error")
			}
		}
	}()

	if err := client.Run(); err != nil {
		log.Fatalf("Slack client error: %v", err)
	}
}
