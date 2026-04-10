package main

import (
	"log"
	"os"
	"regexp"
	"strings"
	"time"

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
	authResp, err := slack.New(cfg.SlackToken).AuthTest()
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

	for {
		slackConnect(cfg, mentionTag, allowedChannels, learnCh, replyCh, helpCh, trainCh)
		log.Printf("Slack: connection lost — reconnecting in 5s")
		time.Sleep(5 * time.Second)
	}
}

func slackConnect(cfg Config, mentionTag string, allowedChannels map[string]bool, learnCh chan<- LearnRequest, replyCh chan<- ReplyRequest, helpCh chan<- HelpRequest, trainCh chan<- TrainRequest) {
	api := slack.New(cfg.SlackToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	client := socketmode.New(api, socketmode.OptionDebug(true), socketmode.OptionLog(log.New(os.Stderr, "socketmode: ", log.Lshortfile|log.LstdFlags)))

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
					if strings.Contains(ev.Text, mentionTag) {
						continue
					}
					if strings.HasPrefix(ev.Text, "&gt;") || strings.HasPrefix(ev.Text, ">") {
						continue
					}
					cleaned := cleanSlackText(ev.Text)
					if cleaned != "" {
						learnCh <- LearnRequest{Text: cleaned}
					}
				case *slackevents.AppMentionEvent:
					log.Printf("Slack: mention user=%s channel=%s", ev.User, ev.Channel)

					if len(allowedChannels) > 0 && !allowedChannels[ev.Channel] {
						info, err := api.GetConversationInfo(&slack.GetConversationInfoInput{
							ChannelID: ev.Channel,
						})
						if err != nil || !allowedChannels[info.Name] {
							continue
						}
					}

					go handleSlackMention(api, ev.Text, ev.Channel, learnCh, replyCh, helpCh, trainCh)
				}

			case socketmode.EventTypeConnecting:
				log.Println("Slack: connecting...")
			case socketmode.EventTypeConnected:
				log.Println("Slack: connected")
			case socketmode.EventTypeConnectionError:
				log.Println("Slack: connection error")
			}
		}
		log.Println("Slack: event channel closed")
	}()

	if err := client.Run(); err != nil {
		log.Printf("Slack: client.Run error: %v", err)
	}
}

func handleSlackMention(api *slack.Client, text, channel string, learnCh chan<- LearnRequest, replyCh chan<- ReplyRequest, helpCh chan<- HelpRequest, trainCh chan<- TrainRequest) {
	cleaned := cleanSlackText(text)
	parsed := parseOverrides(cleaned)

	if parsed.Help {
		rc := make(chan string, 1)
		helpCh <- HelpRequest{ReplyCh: rc}
		reply := <-rc
		_, _, err := api.PostMessage(channel, slack.MsgOptionText(reply, false))
		if err != nil {
			log.Printf("Slack: PostMessage error: %v", err)
		}
		return
	}

	if parsed.TrainURL != "" {
		rc := make(chan string, 1)
		trainCh <- TrainRequest{URL: parsed.TrainURL, ReplyCh: rc}
		reply := <-rc
		_, _, err := api.PostMessage(channel, slack.MsgOptionText(reply, false))
		if err != nil {
			log.Printf("Slack: PostMessage error: %v", err)
		}
		return
	}

	if strings.TrimSpace(parsed.Text) == "" {
		return
	}

	rc := make(chan string, 1)
	replyCh <- ReplyRequest{Text: parsed.Text, Overrides: parsed.Overrides, ReplyCh: rc}
	reply := <-rc
	_, _, err := api.PostMessage(channel, slack.MsgOptionText(reply, false))
	if err != nil {
		log.Printf("Slack: PostMessage error: %v", err)
	}
}
