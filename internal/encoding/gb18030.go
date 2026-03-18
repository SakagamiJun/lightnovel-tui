package encoding

import (
	"bytes"
	"io"
	"net/url"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// GB18030ToUTF8Reader wraps an io.Reader reading GB18030/GBK and translates it to UTF-8 on the fly.
// This operates in a streaming fashion to keep memory consumption minimal.
func GB18030ToUTF8Reader(r io.Reader) io.Reader {
	return transform.NewReader(r, simplifiedchinese.GB18030.NewDecoder())
}

// UTF8ToGB18030Reader wraps a UTF-8 io.Reader and converts it to GB18030.
func UTF8ToGB18030Reader(r io.Reader) io.Reader {
	return transform.NewReader(r, simplifiedchinese.GB18030.NewEncoder())
}

// UTF8ToGBKBytes converts a UTF-8 string into GBK bytes (compatible with Wenku8 search key).
func UTF8ToGBKBytes(s string) ([]byte, error) {
	var buf bytes.Buffer
	w := transform.NewWriter(&buf, simplifiedchinese.GBK.NewEncoder())
	_, err := w.Write([]byte(s))
	if err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// URLEncodeGBK encodes a UTF-8 string into URL-escaped GBK representation (e.g. for search query).
func URLEncodeGBK(s string) (string, error) {
	gbkBytes, err := UTF8ToGBKBytes(s)
	if err != nil {
		return "", err
	}
	// Escape each byte
	return url.QueryEscape(string(gbkBytes)), nil
}
