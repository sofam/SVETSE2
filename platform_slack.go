package main

import (
	"log"
	"os"
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
	client := socketmode.New(api, socketmode.OptionDebug(true), socketmode.OptionLog(log.New(os.Stderr, "socketmode: ", log.Lshortfile|log.LstdFlags)))

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
			log.Printf("Slack event: type=%s", evt.Type)
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					log.Printf("Slack: failed to cast EventsAPI event")
					continue
				}
				client.Ack(*evt.Request)
				log.Printf("Slack: EventsAPI type=%s innerType=%s", eventsAPIEvent.Type, eventsAPIEvent.InnerEvent.Type)

				switch ev := eventsAPIEvent.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					// Only use MessageEvent for learning from non-mention messages.
					// Mentions are handled by AppMentionEvent to avoid double-processing.
					if ev.BotID != "" || ev.SubType != "" {
						continue
					}
					if strings.Contains(ev.Text, mentionTag) {
						continue
					}
					// Skip quoted messages (blockquotes)
					if strings.HasPrefix(ev.Text, "&gt;") || strings.HasPrefix(ev.Text, ">") {
						continue
					}
					cleaned := cleanSlackText(ev.Text)
					if cleaned != "" {
						learnCh <- LearnRequest{Text: cleaned}
					}
				case *slackevents.AppMentionEvent:
					log.Printf("Slack: AppMentionEvent user=%s text=%q channel=%s", ev.User, ev.Text, ev.Channel)

					// Check channel allowlist (fast, do it before spawning goroutine)
					if len(allowedChannels) > 0 && !allowedChannels[ev.Channel] {
						info, err := api.GetConversationInfo(&slack.GetConversationInfoInput{
							ChannelID: ev.Channel,
						})
						if err != nil {
							log.Printf("Slack: GetConversationInfo failed for %s: %v", ev.Channel, err)
							continue
						}
						if !allowedChannels[info.Name] {
							log.Printf("Slack: channel %q (%s) not in allowlist", info.Name, ev.Channel)
							continue
						}
					}

					// Handle in a goroutine so the event loop stays responsive
					go func(text, channel string) {
						cleaned := cleanSlackText(text)
						parsed := parseOverrides(cleaned)

						if parsed.Help {
							rc := make(chan string, 1)
							helpCh <- HelpRequest{ReplyCh: rc}
							reply := <-rc
							api.PostMessage(channel, slack.MsgOptionText(reply, false))
							return
						}

						if parsed.TrainURL != "" {
							rc := make(chan string, 1)
							trainCh <- TrainRequest{URL: parsed.TrainURL, ReplyCh: rc}
							reply := <-rc
							api.PostMessage(channel, slack.MsgOptionText(reply, false))
							return
						}

						if strings.TrimSpace(parsed.Text) == "" {
							return
						}

						rc := make(chan string, 1)
						replyCh <- ReplyRequest{Text: parsed.Text, Overrides: parsed.Overrides, ReplyCh: rc}
						reply := <-rc
						api.PostMessage(channel, slack.MsgOptionText(reply, false))
					}(ev.Text, ev.Channel)

				default:
					log.Printf("Slack: unhandled inner event type: %T", eventsAPIEvent.InnerEvent.Data)
				}

			case socketmode.EventTypeConnecting:
				log.Println("Slack: connecting...")
			case socketmode.EventTypeConnected:
				log.Println("Slack: connected")
			case socketmode.EventTypeConnectionError:
				log.Println("Slack: connection error")
			default:
				log.Printf("Slack: unhandled event type: %s", evt.Type)
			}
		}
	}()

	if err := client.Run(); err != nil {
		log.Fatalf("Slack client error: %v", err)
	}
}
