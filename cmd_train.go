package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var wikiRefRe = regexp.MustCompile(`\[\d+\]`)
var wikiParenRe = regexp.MustCompile(`\s*\([^)]*\)`)
var wikiHeaderRe = regexp.MustCompile(`(?m)^=+.*=+$`)
var wikiExtraWhitespace = regexp.MustCompile(`\s{2,}`)

// runTrain is the "train" subcommand. It loads a brain, trains from sources, and saves.
func runTrain(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `Usage: svetse2 train [sources...]

Sources can be:
  file.txt                    - Train from a text file (one sentence per line)
  https://en.wikipedia.org/wiki/Article  - Fetch and train from a Wikipedia article
  wiki:Article_Name           - Shorthand for Wikipedia article

Options (via environment variables):
  SVETSE2_BRAIN_PATH  - Brain file path (default: ./brain.bin)`)
		os.Exit(1)
	}

	brainPath := envOrDefault("SVETSE2_BRAIN_PATH", "./brain.bin")
	model := newModel(5)

	if err := loadBrain(brainPath, model); err != nil {
		log.Printf("No existing brain loaded (starting fresh): %v", err)
	} else {
		log.Printf("Loaded existing brain: %d words in dictionary", len(model.Dictionary))
	}

	totalSentences := 0

	for _, source := range args {
		var sentences int
		var err error

		switch {
		case strings.HasPrefix(source, "wiki:"):
			lang, articleName := parseWikiShorthand(strings.TrimPrefix(source, "wiki:"))
			sentences, err = trainFromWikipedia(model, lang, articleName)
		case isWikipediaURL(source):
			lang, articleName := extractWikiArticle(source)
			if articleName == "" {
				log.Printf("Could not parse Wikipedia article from URL: %s", source)
				continue
			}
			sentences, err = trainFromWikipedia(model, lang, articleName)
		default:
			sentences, err = trainFromFile(model, source)
		}

		if err != nil {
			log.Printf("Error training from %s: %v", source, err)
			continue
		}

		log.Printf("Trained %d sentences from %s", sentences, source)
		totalSentences += sentences
	}

	if totalSentences == 0 {
		log.Println("No sentences learned, not saving")
		return
	}

	if err := saveBrain(brainPath, model); err != nil {
		log.Fatalf("Error saving brain: %v", err)
	}
	log.Printf("Brain saved: %d words in dictionary, %d sentences trained", len(model.Dictionary), totalSentences)
}

// handleTrain processes a !TRAIN=URL request from chat. Runs in the model goroutine.
func handleTrain(model *Model, source string) string {
	var lang, articleName string
	switch {
	case strings.HasPrefix(source, "wiki:"):
		lang, articleName = parseWikiShorthand(strings.TrimPrefix(source, "wiki:"))
	case isWikipediaURL(source):
		lang, articleName = extractWikiArticle(source)
		if articleName == "" {
			return fmt.Sprintf("Could not parse Wikipedia article from: %s", source)
		}
	default:
		return "!TRAIN supports wiki:Article_Name, wiki:sv:Article, or Wikipedia URLs"
	}

	count, err := trainFromWikipedia(model, lang, articleName)
	if err != nil {
		return fmt.Sprintf("Training failed: %v", err)
	}
	return fmt.Sprintf("Trained %d sentences from %s:%s. Brain now has %d words.", count, lang, articleName, len(model.Dictionary))
}

// parseWikiShorthand parses "Article" or "sv:Article" into (lang, article).
func parseWikiShorthand(s string) (string, string) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 && len(parts[0]) <= 3 {
		return parts[0], parts[1]
	}
	return "en", s
}

// trainFromFile reads a text file and learns each non-empty line.
func trainFromFile(model *Model, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	// Allow long lines (1MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		model.learn(line)
		count++
	}
	return count, scanner.Err()
}

func isWikipediaURL(s string) bool {
	return strings.Contains(s, "wikipedia.org/wiki/")
}

// extractWikiArticle returns (lang, article) from a Wikipedia URL.
func extractWikiArticle(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(u.Path, "/wiki/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	article, err := url.PathUnescape(parts[1])
	if err != nil {
		article = parts[1]
	}
	// Extract language from hostname: sv.wikipedia.org -> sv
	lang := "en"
	hostParts := strings.Split(u.Hostname(), ".")
	if len(hostParts) >= 3 && hostParts[1] == "wikipedia" {
		lang = hostParts[0]
	}
	return lang, article
}

// trainFromWikipedia fetches a Wikipedia article's plain text and learns from it.
func trainFromWikipedia(model *Model, lang, articleName string) (int, error) {
	text, err := fetchWikipediaText(lang, articleName)
	if err != nil {
		return 0, err
	}

	text = cleanWikipediaText(text)
	sentences := splitSentences(text)

	count := 0
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		model.learn(s)
		count++
	}
	return count, nil
}

// fetchWikipediaText fetches the plain text extract of a Wikipedia article using the API.
func fetchWikipediaText(lang, articleName string) (string, error) {
	apiURL := fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&titles=%s&prop=extracts&explaintext=true&format=json",
		lang, url.QueryEscape(articleName),
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "SVETSE2/1.0 (MegaHAL chatbot trainer; https://github.com/oscelf/svetse2)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch Wikipedia: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Wikipedia API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Parse the MediaWiki API response
	var result struct {
		Query struct {
			Pages map[string]struct {
				Title   string `json:"title"`
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	for id, page := range result.Query.Pages {
		if id == "-1" {
			return "", fmt.Errorf("article not found: %s", articleName)
		}
		if page.Extract == "" {
			return "", fmt.Errorf("empty extract for: %s", articleName)
		}
		log.Printf("Fetched Wikipedia article: %s", page.Title)
		return page.Extract, nil
	}

	return "", fmt.Errorf("no pages in response")
}

// cleanWikipediaText removes references [1], section headers, and other wiki cruft.
func cleanWikipediaText(text string) string {
	// Remove [1], [2], etc.
	text = wikiRefRe.ReplaceAllString(text, "")
	// Remove section headers (== Header ==)
	text = wikiHeaderRe.ReplaceAllString(text, "")
	// Collapse excessive whitespace
	text = wikiExtraWhitespace.ReplaceAllString(text, " ")
	return text
}

// splitSentences splits text on sentence boundaries (. ! ?) followed by whitespace.
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		current.WriteRune(runes[i])
		if (runes[i] == '.' || runes[i] == '!' || runes[i] == '?') &&
			i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\n') {
			s := strings.TrimSpace(current.String())
			if s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		}
	}
	// Remaining text
	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}
