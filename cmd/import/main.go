package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"counter/internal/loki"
)

func main() {
	lokiURL := flag.String("loki-url", "http://localhost:3100", "loki base url")
	csvPath := flag.String("file", "", "csv file path (one timestamp per line)")
	dryRun := flag.Bool("dry-run", false, "print what would be imported without sending")
	flag.Parse()

	if *csvPath == "" {
		log.Fatal("missing required -file flag")
	}

	file, err := os.Open(*csvPath)
	if err != nil {
		log.Fatalf("open file: %v", err)
	}
	defer file.Close()

	client := loki.NewClient(*lokiURL)
	labels := map[string]string{"app": "counter"}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	imported := 0
	failed := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		ts, err := parseTimestamp(line)
		if err != nil {
			log.Printf("line %d: invalid timestamp %q: %v", lineNum, line, err)
			failed++
			continue
		}

		if *dryRun {
			fmt.Printf("would import: %s\n", ts.Format(time.RFC3339))
			imported++
			continue
		}

		if err := client.Push(labels, ts, "increment"); err != nil {
			log.Printf("line %d: push failed: %v", lineNum, err)
			failed++
			continue
		}

		imported++
		if imported%100 == 0 {
			log.Printf("imported %d events", imported)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("read file: %v", err)
	}

	log.Printf("done: imported=%d failed=%d", imported, failed)
}

func parseTimestamp(s string) (time.Time, error) {
	formats := []string{
		"02/01/2006,15:04",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized format")
}
