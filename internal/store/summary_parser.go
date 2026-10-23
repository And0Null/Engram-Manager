package store

import (
	"bufio"
	"regexp"
	"strings"
)

// ParsedSummary holds structured sections extracted from a session summary.
type ParsedSummary struct {
	Raw           string            `json:"raw"`
	Goal          string            `json:"goal,omitempty"`
	Accomplished  []string          `json:"accomplished"`
	Decisions     []string          `json:"decisions"`
	Discoveries   []string          `json:"discoveries"`
	NextSteps     []string          `json:"next_steps"`
	OtherSections map[string]string `json:"other_sections,omitempty"`
}

var (
	headerRegex = regexp.MustCompile(`^(?i)#{1,4}\s+(.+)$`)
	bulletRegex = regexp.MustCompile(`^(?i)(?:[-*+]|\d+[.)])\s+(.+)$`)
)

type sectionKind int

const (
	secUnknown sectionKind = iota
	secGoal
	secAccomplished
	secDecisions
	secDiscoveries
	secNextSteps
	secOther
)

// ParseSessionSummary parses markdown session summary into a structured ParsedSummary.
// If the text does not contain canonical headers, it degrades gracefully by placing
// the text in Goal.
func ParseSessionSummary(raw string) ParsedSummary {
	res := ParsedSummary{
		Raw:           raw,
		Accomplished:  []string{},
		Decisions:     []string{},
		Discoveries:   []string{},
		NextSteps:     []string{},
		OtherSections: make(map[string]string),
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return res
	}

	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	currentSec := secUnknown
	currentOtherTitle := ""

	var (
		goalLines      []string
		otherLines     []string
		currentListRef *[]string
		currentBullet  strings.Builder
	)

	flushBullet := func() {
		if currentBullet.Len() > 0 && currentListRef != nil {
			item := strings.TrimSpace(currentBullet.String())
			if item != "" {
				*currentListRef = append(*currentListRef, item)
			}
			currentBullet.Reset()
		}
	}

	flushOther := func() {
		if currentOtherTitle != "" && len(otherLines) > 0 {
			content := strings.TrimSpace(strings.Join(otherLines, "\n"))
			if content != "" {
				res.OtherSections[currentOtherTitle] = content
			}
			otherLines = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Check for header
		if m := headerRegex.FindStringSubmatch(trimmedLine); len(m) > 1 {
			flushBullet()
			flushOther()

			headerTitle := strings.TrimRight(strings.TrimSpace(m[1]), ":")
			normalized := strings.ToLower(headerTitle)

			switch {
			case strings.Contains(normalized, "goal") || strings.Contains(normalized, "objetivo") || strings.Contains(normalized, "task"):
				currentSec = secGoal
				currentListRef = nil
			case strings.Contains(normalized, "accomplished") || strings.Contains(normalized, "shipped") || strings.Contains(normalized, "logros") || strings.Contains(normalized, "what changed"):
				currentSec = secAccomplished
				currentListRef = &res.Accomplished
			case strings.Contains(normalized, "decision") || strings.Contains(normalized, "decisiones"):
				currentSec = secDecisions
				currentListRef = &res.Decisions
			case strings.Contains(normalized, "discover") || strings.Contains(normalized, "learning") || strings.Contains(normalized, "hallazgos"):
				currentSec = secDiscoveries
				currentListRef = &res.Discoveries
			case strings.Contains(normalized, "next") || strings.Contains(normalized, "pasos") || strings.Contains(normalized, "todo"):
				currentSec = secNextSteps
				currentListRef = &res.NextSteps
			default:
				currentSec = secOther
				currentOtherTitle = headerTitle
				currentListRef = nil
			}
			continue
		}

		// Header content processing
		switch currentSec {
		case secGoal:
			if trimmedLine != "" {
				goalLines = append(goalLines, trimmedLine)
			}
		case secAccomplished, secDecisions, secDiscoveries, secNextSteps:
			if bm := bulletRegex.FindStringSubmatch(trimmedLine); len(bm) > 1 {
				flushBullet()
				currentBullet.WriteString(bm[1])
			} else if trimmedLine != "" {
				// Continuation line of bullet or freeform line
				if currentBullet.Len() > 0 {
					currentBullet.WriteString(" ")
					currentBullet.WriteString(trimmedLine)
				} else if currentListRef != nil {
					*currentListRef = append(*currentListRef, trimmedLine)
				}
			}
		case secOther:
			if trimmedLine != "" {
				otherLines = append(otherLines, trimmedLine)
			}
		case secUnknown:
			// If text appears before any header, capture as goal candidate
			if trimmedLine != "" {
				goalLines = append(goalLines, trimmedLine)
			}
		}
	}

	flushBullet()
	flushOther()

	if len(goalLines) > 0 {
		res.Goal = strings.Join(goalLines, " ")
	}

	// Graceful fallback if no sections were identified
	if res.Goal == "" && len(res.Accomplished) == 0 && len(res.Decisions) == 0 &&
		len(res.Discoveries) == 0 && len(res.NextSteps) == 0 && len(res.OtherSections) == 0 {
		res.Goal = trimmed
	}

	return res
}
