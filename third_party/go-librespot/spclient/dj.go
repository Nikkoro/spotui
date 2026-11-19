package spclient

// spotui patch, see third_party/PATCHES.md: DJ's interaction service, which
// takes "Ask DJ" requests and suggests prompts, as Spotify's apps use it.
// Messages from spotify.dj.interactionmanager.v0, encoded by hand to avoid
// generating code for four small messages.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"google.golang.org/protobuf/encoding/protowire"
)

const djInteractionManager = "https://exp.wg.spotify.com/dj-interaction-manager/spotify.dj.interactionmanager.v0.DJInteractionManagerService/"

// DjPromptInput says how a request was entered.
type DjPromptInput int

const (
	DjPromptText      DjPromptInput = 2
	DjPromptSuggested DjPromptInput = 3
)

// DjAskRequest is ResolveInteractiveContextRequest.
type DjAskRequest struct {
	Prompt           string
	SessionId        string
	DeviceId         string
	CurrentSegmentId string
	CurrentTrackUri  string
	Input            DjPromptInput
}

func (r *DjAskRequest) marshal() []byte {
	var b []byte
	b = appendString(b, 1, r.Prompt)
	b = appendString(b, 2, r.SessionId)
	b = appendString(b, 3, r.DeviceId)
	b = appendString(b, 4, r.CurrentSegmentId)
	// 5 retries_left, 6 reason: REQUEST_PROMPT (0) is the default
	b = appendString(b, 7, r.CurrentTrackUri)
	if r.Input != 0 {
		b = protowire.AppendTag(b, 8, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(r.Input))
	}
	return b
}

func appendString(b []byte, num protowire.Number, s string) []byte {
	if s == "" {
		return b
	}
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendString(b, s)
}

// DjAsk asks DJ for something. DJ answers on the named device by playing
// itself again with the segment it put together.
func (c *Spclient) DjAsk(ctx context.Context, req *DjAskRequest) error {
	body, err := c.djInteraction(ctx, "ResolveInteractiveContext", req.marshal())
	if err != nil {
		return err
	}
	status, message, err := parseStrings12(body)
	if err != nil {
		return fmt.Errorf("parsing DJ answer: %w", err)
	}
	if status != "OK" {
		return fmt.Errorf("DJ turned the request down: %s %s", status, message)
	}
	return nil
}

// DjSuggestions returns the prompts Spotify's apps suggest while trackUri plays.
func (c *Spclient) DjSuggestions(ctx context.Context, trackUri string) ([]string, error) {
	body, err := c.djInteraction(ctx, "GetSuggestedPrompts", appendString(nil, 3, trackUri))
	if err != nil {
		return nil, err
	}
	var prompts []string
	for len(body) > 0 {
		num, typ, n := protowire.ConsumeTag(body)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		body = body[n:]
		if num == 1 && typ == protowire.BytesType {
			s, m := protowire.ConsumeString(body)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			prompts = append(prompts, s)
			body = body[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, body)
		if m < 0 {
			return nil, protowire.ParseError(m)
		}
		body = body[m:]
	}
	return prompts, nil
}

// parseStrings12 reads string fields 1 and 2 of a message.
func parseStrings12(b []byte) (f1, f2 string, err error) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", "", protowire.ParseError(n)
		}
		b = b[n:]
		if typ == protowire.BytesType && (num == 1 || num == 2) {
			s, m := protowire.ConsumeString(b)
			if m < 0 {
				return "", "", protowire.ParseError(m)
			}
			if num == 1 {
				f1 = s
			} else {
				f2 = s
			}
			b = b[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			return "", "", protowire.ParseError(m)
		}
		b = b[m:]
	}
	return f1, f2, nil
}

func (c *Spclient) djInteraction(ctx context.Context, method string, body []byte) ([]byte, error) {
	u, err := url.Parse(djInteractionManager + method)
	if err != nil {
		return nil, err
	}
	// Not retried: a request DJ already acted on must not be sent twice.
	resp, err := c.innerRequestWith(ctx, c.client, http.MethodPost, u, nil, nil, body, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DJ %s: status %d", method, resp.StatusCode)
	}
	return data, nil
}
