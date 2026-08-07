package bridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxFrameSize = 64 << 20

type RPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []any  `json:"details,omitempty"`
}

func (e *RPCError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

func (c *Client) Call(ctx context.Context, service, method string, requestValue, responseValue any) error {
	body, err := json.Marshal(requestValue)
	if err != nil {
		return fmt.Errorf("marshal %s/%s request: %w", service, method, err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.rpcURL(service, method),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("create %s/%s request: %w", service, method, err)
	}
	c.setHeaders(request, "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call %s/%s: %w", service, method, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return decodeRPCError(response.Body, response.Status)
	}
	if responseValue == nil {
		_, err := io.Copy(io.Discard, response.Body)
		return err
	}
	if err := json.NewDecoder(response.Body).Decode(responseValue); err != nil {
		return fmt.Errorf("decode %s/%s response: %w", service, method, err)
	}
	return nil
}

func (c *Client) Stream(ctx context.Context, service, method string, requestValue any) (*StreamReader, error) {
	payload, err := json.Marshal(requestValue)
	if err != nil {
		return nil, fmt.Errorf("marshal %s/%s request: %w", service, method, err)
	}
	var body bytes.Buffer
	if err := writeFrame(&body, 0x00, payload); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.rpcURL(service, method),
		&body,
	)
	if err != nil {
		return nil, fmt.Errorf("create %s/%s stream request: %w", service, method, err)
	}
	c.setHeaders(request, "application/connect+json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("stream %s/%s: %w", service, method, err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		return nil, decodeRPCError(response.Body, response.Status)
	}
	return &StreamReader{body: response.Body, maxFrameSize: maxFrameSize}, nil
}

func (c *Client) rpcURL(service, method string) string {
	return c.baseURL + "/sdk.v1." + service + "/" + method
}

func (c *Client) setHeaders(request *http.Request, contentType string) {
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Connect-Protocol-Version", "1")
}

func decodeRPCError(reader io.Reader, status string) error {
	var rpcError RPCError
	if err := json.NewDecoder(reader).Decode(&rpcError); err != nil {
		return fmt.Errorf("bridge RPC returned %s", status)
	}
	if rpcError.Message == "" {
		rpcError.Message = "bridge RPC returned " + status
	}
	return &rpcError
}

type StreamReader struct {
	body         io.ReadCloser
	maxFrameSize uint32
	done         bool
}

func (r *StreamReader) Next(message any) error {
	if r.done {
		return io.EOF
	}
	for {
		flags, payload, err := readFrame(r.body, r.maxFrameSize)
		if err != nil {
			return err
		}
		switch flags {
		case 0x00:
			if err := json.Unmarshal(payload, message); err != nil {
				return fmt.Errorf("decode stream message: %w", err)
			}
			return nil
		case 0x02:
			r.done = true
			_ = r.body.Close()
			var end struct {
				Error *RPCError `json:"error"`
			}
			if err := json.Unmarshal(payload, &end); err != nil {
				return fmt.Errorf("decode stream end: %w", err)
			}
			if end.Error != nil {
				return end.Error
			}
			return io.EOF
		default:
			return fmt.Errorf("unsupported Connect frame flags 0x%02x", flags)
		}
	}
}

func (r *StreamReader) Close() error {
	r.done = true
	return r.body.Close()
}

func writeFrame(writer io.Writer, flags byte, payload []byte) error {
	if len(payload) > maxFrameSize {
		return fmt.Errorf("Connect frame size %d exceeds limit %d", len(payload), maxFrameSize)
	}
	var header [5]byte
	header[0] = flags
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := writer.Write(header[:]); err != nil {
		return fmt.Errorf("write Connect frame header: %w", err)
	}
	if _, err := writer.Write(payload); err != nil {
		return fmt.Errorf("write Connect frame payload: %w", err)
	}
	return nil
}

func readFrame(reader io.Reader, limit uint32) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(header[1:])
	if size > limit {
		return 0, nil, fmt.Errorf("Connect frame size %d exceeds limit %d", size, limit)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, fmt.Errorf("read Connect frame payload: %w", err)
	}
	return header[0], payload, nil
}
