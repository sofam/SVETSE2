package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	SlackToken    string
	SlackAppToken string
	DiscordToken  string

	SlackChannels   []string
	DiscordChannels []string

	BrainPath    string
	SaveInterval time.Duration

	BanFile string
	AuxFile string
	SwpFile string

	DefaultConfig GenerationConfig
}

func loadConfig() Config {
	cfg := Config{
		SlackToken:    os.Getenv("SVETSE2_SLACK_TOKEN"),
		SlackAppToken: os.Getenv("SVETSE2_SLACK_APP_TOKEN"),
		DiscordToken:  os.Getenv("SVETSE2_DISCORD_TOKEN"),
		BrainPath:     envOrDefault("SVETSE2_BRAIN_PATH", "./brain.bin"),
		BanFile:       envOrDefault("SVETSE2_BAN_FILE", "./megahal.ban"),
		AuxFile:       envOrDefault("SVETSE2_AUX_FILE", "./megahal.aux"),
		SwpFile:       envOrDefault("SVETSE2_SWP_FILE", "./megahal.swp"),
	}

	if ch := os.Getenv("SVETSE2_SLACK_CHANNELS"); ch != "" {
		cfg.SlackChannels = strings.Split(ch, ",")
		for i := range cfg.SlackChannels {
			cfg.SlackChannels[i] = strings.TrimSpace(cfg.SlackChannels[i])
		}
	}
	if ch := os.Getenv("SVETSE2_DISCORD_CHANNELS"); ch != "" {
		cfg.DiscordChannels = strings.Split(ch, ",")
		for i := range cfg.DiscordChannels {
			cfg.DiscordChannels[i] = strings.TrimSpace(cfg.DiscordChannels[i])
		}
	}

	cfg.SaveInterval = parseDurationEnv("SVETSE2_SAVE_INTERVAL", 5*time.Minute)
	chaos := parseFloatEnv("SVETSE2_CHAOS", 1.0)
	cfg.DefaultConfig = GenerationConfig{
		Temperature:  parseFloatEnv("SVETSE2_TEMPERATURE", chaos),
		SurpriseBias: parseFloatEnv("SVETSE2_SURPRISE_BIAS", chaos),
		ReplyTimeout: parseDurationEnv("SVETSE2_REPLY_TIMEOUT", 2*time.Second),
	}

	return cfg
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseFloatEnv(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%f", &f); err != nil {
		return def
	}
	return f
}

func parseDurationEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

type LearnRequest struct {
	Text string
}

type ReplyRequest struct {
	Text      string
	Overrides map[string]string
	ReplyCh   chan string
}

type HelpRequest struct {
	ReplyCh chan string
}

type TrainRequest struct {
	URL     string
	ReplyCh chan string
}

func helpText(cfg GenerationConfig) string {
	return fmt.Sprintf("SVETSE2 — MegaHAL Markov chain bot\n\nUsage: @bot <message> [!KEY=VALUE...]\n\nPer-message overrides:\n  !CHAOS=X          Combined chaos dial (default: %.1f)\n  !TEMPERATURE=X    Random walk temperature (default: %.1f)\n  !SURPRISE_BIAS=X  Surprise scoring exponent (default: %.1f)\n  !TIMEOUT=Xs       Reply generation time (default: %s, max: 30s)\n  !TRAIN=URL        Train from Wikipedia (wiki:Article or full URL)\n  !HELP             Show this message\n\nHigher CHAOS = wilder, more unhinged replies.",
		cfg.Temperature, cfg.Temperature, cfg.SurpriseBias, cfg.ReplyTimeout)
}

func runModelGoroutine(cfg Config, learnCh <-chan LearnRequest, replyCh <-chan ReplyRequest, helpCh <-chan HelpRequest, trainCh <-chan TrainRequest, quit <-chan struct{}) {
	model := newModel(5)
	ban := loadWordList(cfg.BanFile)
	aux := loadWordList(cfg.AuxFile)
	swaps := loadSwapList(cfg.SwpFile)

	if err := loadBrain(cfg.BrainPath, model); err != nil {
		log.Printf("No existing brain loaded: %v", err)
	} else {
		log.Printf("Brain loaded: %d words in dictionary", len(model.Dictionary))
	}

	saveTicker := time.NewTicker(cfg.SaveInterval)
	defer saveTicker.Stop()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	save := func() {
		if err := saveBrain(cfg.BrainPath, model); err != nil {
			log.Printf("Error saving brain: %v", err)
		} else {
			log.Printf("Brain saved: %d words in dictionary", len(model.Dictionary))
		}
	}

	for {
		select {
		case req := <-learnCh:
			model.learn(req.Text)
		case req := <-replyCh:
			// Run reply generation in a goroutine with a hard timeout
			// to prevent the model goroutine from blocking forever
			done := make(chan string, 1)
			go func() {
				genCfg := applyOverrides(cfg.DefaultConfig, req.Overrides)
				done <- model.generateReply(req.Text, ban, aux, swaps, genCfg)
			}()
			select {
			case reply := <-done:
				req.ReplyCh <- reply
			case <-time.After(60 * time.Second):
				req.ReplyCh <- "Brain timed out generating a reply."
				log.Printf("WARNING: generateReply timed out for input %q", req.Text)
			}
		case req := <-helpCh:
			req.ReplyCh <- helpText(cfg.DefaultConfig)
		case req := <-trainCh:
			req.ReplyCh <- handleTrain(model, req.URL)
		case <-heartbeat.C:
			log.Printf("heartbeat: dict=%d learnQ=%d/%d replyQ=%d/%d",
				len(model.Dictionary), len(learnCh), cap(learnCh), len(replyCh), cap(replyCh))
		case <-saveTicker.C:
			save()
		case <-quit:
			save()
			return
		}
	}
}

func main() {
	// Subcommand routing
	if len(os.Args) > 1 && os.Args[1] == "train" {
		runTrain(os.Args[2:])
		return
	}

	cfg := loadConfig()

	if cfg.SlackToken == "" && cfg.DiscordToken == "" {
		log.Fatal("At least one of SVETSE2_SLACK_TOKEN or SVETSE2_DISCORD_TOKEN must be set")
	}

	learnCh := make(chan LearnRequest, 100)
	replyCh := make(chan ReplyRequest, 10)
	helpCh := make(chan HelpRequest, 10)
	trainCh := make(chan TrainRequest, 5)
	quit := make(chan struct{})

	// pprof debug endpoint — ssh tunnel to inspect goroutine stacks when stuck
	go func() {
		log.Println("pprof listening on localhost:6060")
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	go runModelGoroutine(cfg, learnCh, replyCh, helpCh, trainCh, quit)

	if cfg.SlackToken != "" {
		go runSlack(cfg, learnCh, replyCh, helpCh, trainCh)
		log.Println("Slack adapter started")
	}

	if cfg.DiscordToken != "" {
		go runDiscord(cfg, learnCh, replyCh, helpCh, trainCh)
		log.Println("Discord adapter started")
	}

	log.Println("SVETSE2 running. Press Ctrl+C to stop.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
	close(quit)
	time.Sleep(2 * time.Second)
}
