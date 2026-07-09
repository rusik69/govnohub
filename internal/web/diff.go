package web

import (
	"strconv"
	"strings"
)

type DiffLine struct {
	Type    string // "hunk", "add", "del", "ctx"
	OldLine int
	NewLine int
	Content string
}

type DiffFile struct {
	OldPath string
	NewPath string
	Lines   []DiffLine
}

func ParseUnifiedDiff(diff string) []DiffFile {
	var files []DiffFile
	var cur *DiffFile
	oldLine, newLine := 0, 0

	for _, raw := range strings.Split(diff, "\n") {
		line := raw
		if strings.HasPrefix(line, "diff --git") {
			if cur != nil {
				files = append(files, *cur)
			}
			cur = &DiffFile{}
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(line, "--- ") {
			cur.OldPath = strings.TrimPrefix(line, "--- ")
			if i := strings.Index(cur.OldPath, "\t"); i >= 0 {
				cur.OldPath = cur.OldPath[:i]
			}
			cur.OldPath = strings.TrimPrefix(cur.OldPath, "a/")
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			cur.NewPath = strings.TrimPrefix(line, "+++ ")
			if i := strings.Index(cur.NewPath, "\t"); i >= 0 {
				cur.NewPath = cur.NewPath[:i]
			}
			cur.NewPath = strings.TrimPrefix(cur.NewPath, "b/")
			continue
		}
		if strings.HasPrefix(line, "@@") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				oldLine = parseHunkStart(parts[1])
				newLine = parseHunkStart(parts[2])
			}
			cur.Lines = append(cur.Lines, DiffLine{Type: "hunk", Content: line})
			continue
		}
		if len(line) == 0 {
			cur.Lines = append(cur.Lines, DiffLine{Type: "ctx", OldLine: oldLine, NewLine: newLine, Content: ""})
			oldLine++
			newLine++
			continue
		}
		switch line[0] {
		case '+':
			cur.Lines = append(cur.Lines, DiffLine{Type: "add", NewLine: newLine, Content: line[1:]})
			newLine++
		case '-':
			cur.Lines = append(cur.Lines, DiffLine{Type: "del", OldLine: oldLine, Content: line[1:]})
			oldLine++
		case ' ':
			cur.Lines = append(cur.Lines, DiffLine{Type: "ctx", OldLine: oldLine, NewLine: newLine, Content: line[1:]})
			oldLine++
			newLine++
		default:
			cur.Lines = append(cur.Lines, DiffLine{Type: "ctx", Content: line})
		}
	}
	if cur != nil {
		files = append(files, *cur)
	}
	return files
}

func parseHunkStart(s string) int {
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "-")
	if i := strings.Index(s, ","); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	if n < 0 {
		return 0
	}
	return n
}
