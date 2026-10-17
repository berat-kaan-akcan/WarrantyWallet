package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3 stores files in an S3-compatible bucket (AWS S3, MinIO, Cloudflare R2, ...).
// Requests are signed with AWS Signature Version 4 using only the standard library.
type S3 struct {
	Endpoint  string // e.g. https://s3.eu-central-1.amazonaws.com or http://minio:9000
	Bucket    string
	Region    string // default us-east-1 (MinIO's default)
	AccessKey string
	SecretKey string
	// PathStyle uses endpoint/bucket/key (MinIO default). When false, bucket.endpoint/key is used.
	PathStyle bool
	// MaxSize caps Put bodies; uploads are buffered in memory to sign the payload. Default 32 MB.
	MaxSize int64
	Client  *http.Client
	Now     func() time.Time
}

// NewS3 validates the configuration.
func NewS3(s S3) (*S3, error) {
	u, err := url.Parse(s.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid S3 endpoint %q", s.Endpoint)
	}
	if s.Bucket == "" || s.AccessKey == "" || s.SecretKey == "" {
		return nil, fmt.Errorf("S3 bucket, access key and secret key are required")
	}
	if s.Region == "" {
		s.Region = "us-east-1"
	}
	if s.MaxSize == 0 {
		s.MaxSize = 32 << 20
	}
	if s.Client == nil {
		s.Client = &http.Client{Timeout: 60 * time.Second}
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	s.Endpoint = strings.TrimRight(s.Endpoint, "/")
	return &s, nil
}

func (s *S3) objectURL(key string) (string, error) {
	if !validKey.MatchString(key) || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	segs := strings.Split(key, "/")
	for i, p := range segs {
		segs[i] = url.PathEscape(p)
	}
	path := strings.Join(segs, "/")
	if s.PathStyle {
		return s.Endpoint + "/" + url.PathEscape(s.Bucket) + "/" + path, nil
	}
	u, _ := url.Parse(s.Endpoint)
	u.Host = s.Bucket + "." + u.Host
	return u.String() + "/" + path, nil
}

func (s *S3) do(ctx context.Context, method, key string, body []byte) (*http.Response, error) {
	u, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if body == nil {
		req.Body = http.NoBody
	}
	req.ContentLength = int64(len(body))
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	s.sign(req, body)
	return s.Client.Do(req)
}

// Put uploads r (at most MaxSize bytes) to key.
func (s *S3) Put(ctx context.Context, key string, r io.Reader) (int64, error) {
	data, err := io.ReadAll(io.LimitReader(r, s.MaxSize+1))
	if err != nil {
		return 0, err
	}
	if int64(len(data)) > s.MaxSize {
		return 0, fmt.Errorf("object larger than %d bytes", s.MaxSize)
	}
	res, err := s.do(ctx, http.MethodPut, key, data)
	if err != nil {
		return 0, err
	}
	defer drain(res)
	if res.StatusCode/100 != 2 {
		return 0, s3Error("put", res)
	}
	return int64(len(data)), nil
}

// Open streams key; the caller closes the reader.
func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	res, err := s.do(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusOK:
		return res.Body, nil
	case res.StatusCode == http.StatusNotFound:
		drain(res)
		return nil, ErrNotFound
	default:
		defer drain(res)
		return nil, s3Error("get", res)
	}
}

// Delete removes key; S3 answers 204 for missing keys too.
func (s *S3) Delete(ctx context.Context, key string) error {
	res, err := s.do(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	defer drain(res)
	if res.StatusCode/100 != 2 && res.StatusCode != http.StatusNotFound {
		return s3Error("delete", res)
	}
	return nil
}

func drain(res *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	_ = res.Body.Close()
}

// s3Error reports the status and the XML error code, never the request (no credentials).
func s3Error(op string, res *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
	code := ""
	if i := bytes.Index(b, []byte("<Code>")); i >= 0 {
		rest := b[i+6:]
		if j := bytes.Index(rest, []byte("</Code>")); j >= 0 && j < 100 {
			code = string(rest[:j])
		}
	}
	return fmt.Errorf("s3 %s: HTTP %d %s", op, res.StatusCode, code)
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sign adds AWS SigV4 headers. Every header already on the request is signed, plus host.
func (s *S3) sign(req *http.Request, body []byte) {
	now := s.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	payload := emptySHA256
	if len(body) > 0 {
		payload = hexSHA256(body)
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payload)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery(req.URL.Query()),
		canonHeaders.String(),
		signed,
		payload,
	}, "\n")
	scope := day + "/" + s.Region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hexSHA256([]byte(canonical))
	k := hmacSHA256([]byte("AWS4"+s.SecretKey), day)
	k = hmacSHA256(k, s.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func hexSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}
