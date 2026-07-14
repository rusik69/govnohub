package web

import (
	"fmt"
	"strings"
)

// badgeSVG generates a shields.io-style SVG badge with the given label and value.
// Colors: success=green, failure=red, cancelled=gray, other=yellow.
func badgeSVG(label, value, color string) string {
	label = strings.ToUpper(label)
	// Calculate approximate text widths (rough: ~7px per char for 11px font)
	labelW := len(label)*7 + 14
	valueW := len(value)*7 + 14
	totalW := labelW + valueW
	labelMid := labelW / 2
	valueMid := labelW + valueW/2

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
  <linearGradient id="s" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`, totalW, label, value,
		totalW,
		labelW, labelW, valueW, color,
		totalW,
		labelMid, label, labelMid, label,
		valueMid, value, valueMid, value)
}

// badgeColor returns the badge color for a workflow run conclusion/status.
func badgeColor(conclusion, status string) string {
	if conclusion == "success" {
		return "#1a7f37" // green
	}
	if conclusion == "failure" {
		return "#cf222e" // red
	}
	if conclusion == "cancelled" || conclusion == "skipped" {
		return "#656d76" // gray
	}
	if status == "in_progress" || status == "queued" || status == "pending" {
		return "#bf8700" // yellow/amber
	}
	return "#656d76" // gray for unknown
}

// badgeValue returns the human-readable badge value for a workflow run.
func badgeValue(conclusion, status string) string {
	if conclusion == "success" {
		return "passing"
	}
	if conclusion == "failure" {
		return "failing"
	}
	if conclusion == "cancelled" {
		return "cancelled"
	}
	if status == "in_progress" {
		return "running"
	}
	if status == "queued" || status == "pending" || status == "waiting" {
		return "pending"
	}
	return "unknown"
}
