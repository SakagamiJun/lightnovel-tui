package encoding

import (
	"bytes"
	"io"
	"testing"
)

func TestGB18030Conversion(t *testing.T) {
	orig := "关于我转生变成史莱姆这档事 • 特殊字符测试 〜"
	// Encode to GB18030
	var buf bytes.Buffer
	encReader := UTF8ToGB18030Reader(bytes.NewBufferString(orig))
	_, err := io.Copy(&buf, encReader)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	// Decode back to UTF-8
	decReader := GB18030ToUTF8Reader(&buf)
	decodedBytes, err := io.ReadAll(decReader)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if string(decodedBytes) != orig {
		t.Errorf("expected %q, got %q", orig, string(decodedBytes))
	}
}

func TestURLEncodeGBK(t *testing.T) {
	query := "史莱姆"
	encoded, err := URLEncodeGBK(query)
	if err != nil {
		t.Fatalf("failed to urlencode: %v", err)
	}
	if len(encoded) == 0 {
		t.Errorf("expected non-empty encoded query")
	}
}
