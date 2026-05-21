package core

import "strings"

type DebtMarker struct {
	Marker   string
	Text     string
	Location Location
}

var debtMarkerKinds = []string{"TODO", "FIXME", "HACK", "XXX", "DEPRECATED"}

func FindDebtMarkersInRange(path string, src []byte, start, end int) []DebtMarker {
	if start < 0 {
		start = 0
	}
	if end > len(src) {
		end = len(src)
	}
	if start >= end {
		return nil
	}
	return findDebtMarkers(path, src, start, end)
}

func FindDebtMarkers(path string, src []byte) []DebtMarker {
	return findDebtMarkers(path, src, 0, len(src))
}

func FindMarkdownDebtMarkers(path string, src []byte) []DebtMarker {
	var out []DebtMarker
	inFence := false
	lineStart := 0
	for lineStart <= len(src) {
		lineEnd := lineStart
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}
		line := string(src[lineStart:lineEnd])
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		} else if !inFence {
			out = append(out, findMarkdownLineDebtMarkers(path, src, lineStart, lineEnd)...)
		}
		if lineEnd == len(src) {
			break
		}
		lineStart = lineEnd + 1
	}
	return out
}

func findMarkdownLineDebtMarkers(path string, src []byte, lineStart, lineEnd int) []DebtMarker {
	line := string(src[lineStart:lineEnd])
	masked := maskMarkdownInlineCode(line)
	upper := strings.ToUpper(masked)
	var out []DebtMarker
	for _, marker := range debtMarkerKinds {
		idx := debtMarkerIndex(upper, marker)
		if idx < 0 {
			continue
		}
		markerStart := lineStart + idx
		markerEnd := markerStart + len(marker)
		out = append(out, DebtMarker{
			Marker: marker,
			Text:   strings.TrimSpace(line),
			Location: Location{
				URI: path,
				Range: Range{
					Start: PositionForOffset(src, markerStart),
					End:   PositionForOffset(src, markerEnd),
				},
			},
		})
	}
	return out
}

func maskMarkdownInlineCode(line string) string {
	var b strings.Builder
	b.Grow(len(line))
	inCode := false
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			inCode = !inCode
			b.WriteByte(' ')
			continue
		}
		if inCode {
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(line[i])
	}
	return b.String()
}

func CountDebtMarkers(markers []DebtMarker) map[string]int {
	out := map[string]int{}
	for _, marker := range markers {
		out[marker.Marker]++
	}
	return out
}

func findDebtMarkers(path string, src []byte, start, end int) []DebtMarker {
	var out []DebtMarker
	lineStart := start
	for lineStart <= end {
		lineEnd := lineStart
		for lineEnd < end && src[lineEnd] != '\n' {
			lineEnd++
		}
		line := string(src[lineStart:lineEnd])
		upper := strings.ToUpper(line)
		for _, marker := range debtMarkerKinds {
			idx := debtMarkerIndex(upper, marker)
			if idx < 0 {
				continue
			}
			markerStart := lineStart + idx
			markerEnd := markerStart + len(marker)
			out = append(out, DebtMarker{
				Marker: marker,
				Text:   strings.TrimSpace(line),
				Location: Location{
					URI: path,
					Range: Range{
						Start: PositionForOffset(src, markerStart),
						End:   PositionForOffset(src, markerEnd),
					},
				},
			})
		}
		if lineEnd == end {
			break
		}
		lineStart = lineEnd + 1
	}
	return out
}

func debtMarkerIndex(s, marker string) int {
	offset := 0
	for {
		idx := strings.Index(s[offset:], marker)
		if idx < 0 {
			return -1
		}
		idx += offset
		beforeOK := idx == 0 || !isDebtMarkerWordByte(s[idx-1])
		after := idx + len(marker)
		afterOK := after >= len(s) || !isDebtMarkerWordByte(s[after])
		if beforeOK && afterOK {
			return idx
		}
		offset = idx + len(marker)
	}
}

func isDebtMarkerWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z'
}
