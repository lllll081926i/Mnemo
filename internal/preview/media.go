package preview

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

func isHLSPlaylist(streamType, contentType, target string, rootRequest bool) bool {
	if rootRequest && strings.EqualFold(strings.TrimSpace(streamType), "hls") {
		return true
	}
	contentType = strings.ToLower(contentType)
	return strings.Contains(contentType, "mpegurl") || strings.Contains(strings.ToLower(target), ".m3u8")
}

func isDASHManifest(streamType, contentType, target string, rootRequest bool) bool {
	if rootRequest && strings.EqualFold(strings.TrimSpace(streamType), "dash") {
		return true
	}
	contentType = strings.ToLower(contentType)
	path := strings.ToLower(strings.SplitN(target, "?", 2)[0])
	return strings.Contains(contentType, "dash+xml") || strings.HasSuffix(path, ".mpd")
}

func isSRTSubtitle(streamType, contentType, target string) bool {
	if strings.EqualFold(strings.TrimSpace(streamType), "subtitle-srt") {
		return true
	}
	contentType = strings.ToLower(contentType)
	path := strings.ToLower(strings.SplitN(target, "?", 2)[0])
	return strings.Contains(contentType, "subrip") || strings.HasSuffix(path, ".srt")
}

func readPlaylistBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}
	return io.ReadAll(io.LimitReader(reader, 8<<20))
}

func (s *Server) rewriteHLSPlaylist(sessionID string, session *playbackSession, playlistURL string, body []byte) ([]byte, error) {
	base, err := url.Parse(playlistURL)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(body), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			lines[index] = rewriteHLSURIAttributes(line, func(raw string) string {
				return s.hlsResourceURL(sessionID, session, resolveHLSReference(base, raw))
			})
			continue
		}
		lines[index] = s.hlsResourceURL(sessionID, session, resolveHLSReference(base, trimmed))
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func (s *Server) hlsResourceURL(sessionID string, session *playbackSession, target string) string {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		// HLS permits data/blob-like URI attributes (for example, identity
		// key formats). They are not network resources and must remain intact.
		return target
	}
	return s.playbackResourceURL(sessionID, session, target)
}

func resolveHLSReference(base *url.URL, raw string) string {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	return base.ResolveReference(ref).String()
}

func rewriteHLSURIAttributes(line string, replacement func(string) string) string {
	upper := strings.ToUpper(line)
	for start := 0; ; {
		index := strings.Index(upper[start:], "URI=")
		if index < 0 {
			return line
		}
		index += start + len("URI=")
		if index >= len(line) {
			return line
		}
		if line[index] == '"' {
			end := strings.Index(line[index+1:], "\"")
			if end < 0 {
				return line
			}
			end += index + 1
			line = line[:index+1] + replacement(line[index+1:end]) + line[end:]
			upper = strings.ToUpper(line)
			start = end + 1
			continue
		}
		end := len(line)
		if comma := strings.IndexByte(line[index:], ','); comma >= 0 {
			end = index + comma
		}
		line = line[:index] + replacement(line[index:end]) + line[end:]
		upper = strings.ToUpper(line)
		start = end
	}
}

// rewriteDASHManifest replaces external segment references with local /stream/
// routes. DASH requests are generated from XML templates after parsing, so the
// HLS-style one-resource-per-complete-URL approach would leave relative
// segments pointing at an invalid local path or directly at the provider.
func (s *Server) rewriteDASHManifest(sessionID string, session *playbackSession, manifestURL string, body []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)

	type elementState struct {
		name         xml.Name
		isBaseURL    bool
		isTextURL    bool
		baseResolved bool
	}
	bases := []string{manifestURL}
	elements := make([]elementState, 0, 8)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse DASH manifest: %w", err)
		}

		switch value := token.(type) {
		case xml.StartElement:
			currentBase := bases[len(bases)-1]
			for index := range value.Attr {
				if !isDASHURLAttribute(value.Name.Local, value.Attr[index].Name.Local) || strings.TrimSpace(value.Attr[index].Value) == "" {
					continue
				}
				rewritten, err := s.rewriteDASHReference(sessionID, session, currentBase, value.Attr[index].Value)
				if err != nil {
					return nil, err
				}
				value.Attr[index].Value = rewritten
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
			isBaseURL := strings.EqualFold(value.Name.Local, "BaseURL")
			elements = append(elements, elementState{
				name:      value.Name,
				isBaseURL: isBaseURL,
				isTextURL: isBaseURL || strings.EqualFold(value.Name.Local, "Location") || strings.EqualFold(value.Name.Local, "PatchLocation"),
			})
			bases = append(bases, currentBase)

		case xml.CharData:
			if len(elements) > 0 {
				state := &elements[len(elements)-1]
				if state.isTextURL && (!state.isBaseURL || !state.baseResolved) {
					raw := string(value)
					trimmed := strings.TrimSpace(raw)
					if trimmed != "" {
						baseIndex := len(bases) - 1
						if state.isBaseURL {
							baseIndex--
						}
						resolved, ok := resolveDASHReference(bases[baseIndex], trimmed)
						if ok {
							rewritten, err := s.dashResourceURL(sessionID, session, resolved)
							if err != nil {
								return nil, err
							}
							if state.isBaseURL {
								bases[baseIndex] = resolved
							}
							leading := raw[:len(raw)-len(strings.TrimLeft(raw, " \t\r\n"))]
							trailing := raw[len(strings.TrimRight(raw, " \t\r\n")):]
							value = xml.CharData([]byte(leading + rewritten + trailing))
						}
						state.baseResolved = true
					}
				}
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}

		case xml.EndElement:
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
			if len(elements) == 0 || len(bases) < 2 {
				return nil, fmt.Errorf("invalid DASH manifest structure")
			}
			elements = elements[:len(elements)-1]
			bases = bases[:len(bases)-1]

		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func isDASHURLAttribute(element, attribute string) bool {
	attribute = strings.ToLower(attribute)
	switch attribute {
	case "media", "initialization", "index", "sourceurl", "href":
		return true
	case "value":
		return strings.EqualFold(element, "UTCTiming")
	default:
		return false
	}
}

func (s *Server) rewriteDASHReference(sessionID string, session *playbackSession, base, raw string) (string, error) {
	target, ok := resolveDASHReference(base, raw)
	if !ok {
		return raw, nil
	}
	return s.dashResourceURL(sessionID, session, target)
}

func resolveDASHReference(base, raw string) (string, bool) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", false
	}
	reference, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	target := baseURL.ResolveReference(reference)
	if (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" {
		return "", false
	}
	return target.String(), true
}

// srtToWebVTT converts the portable SubRip form that providers commonly
// expose into the format supported by the browser <track> element. The
// conversion deliberately leaves cue text untouched and only normalizes line
// endings, cue indices and timestamp separators.
func srtToWebVTT(body []byte) []byte {
	text := strings.TrimPrefix(string(body), "\ufeff")
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") {
		return []byte(text)
	}
	lines := strings.Split(text, "\n")
	var output strings.Builder
	output.WriteString("WEBVTT\n\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isSubtitleCueIndex(trimmed) && index+1 < len(lines) && strings.Contains(lines[index+1], "-->") {
			continue
		}
		if strings.Contains(line, "-->") {
			line = strings.ReplaceAll(line, ",", ".")
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	return []byte(output.String())
}

func isSubtitleCueIndex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// checkProxyRedirect validates redirect targets to prevent the proxy from
// being redirected to private addresses.

func contentTypeFor(path string) string {
	if typ := mime.TypeByExtension(filepath.Ext(path)); typ != "" {
		return typ
	}
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".mp4"), strings.HasSuffix(lower, ".m4v"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".webm"):
		return "video/webm"
	case strings.HasSuffix(lower, ".mkv"):
		return "video/x-matroska"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".m4a"), strings.HasSuffix(lower, ".aac"):
		return "audio/mp4"
	case strings.HasSuffix(lower, ".flac"):
		return "audio/flac"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	case strings.HasSuffix(lower, ".ogg"):
		return "audio/ogg"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lower, ".ico"):
		return "image/x-icon"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".log"),
		strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".vue"),
		strings.HasSuffix(lower, ".json"), strings.HasSuffix(lower, ".go"), strings.HasSuffix(lower, ".py"),
		strings.HasSuffix(lower, ".java"), strings.HasSuffix(lower, ".c"), strings.HasSuffix(lower, ".cpp"),
		strings.HasSuffix(lower, ".h"), strings.HasSuffix(lower, ".css"), strings.HasSuffix(lower, ".html"),
		strings.HasSuffix(lower, ".xml"), strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"),
		strings.HasSuffix(lower, ".ini"), strings.HasSuffix(lower, ".sh"), strings.HasSuffix(lower, ".bat"),
		strings.HasSuffix(lower, ".srt"), strings.HasSuffix(lower, ".vtt"), strings.HasSuffix(lower, ".ass"):
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// Close shuts the server down.
