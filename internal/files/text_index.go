package files

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
)

const MaxIndexedTextBytes = 512 * 1024

func SupportsTextIndex(previewKind string) bool {
	switch previewKind {
	case "text", "markdown", "pdf":
		return true
	}
	return false
}

func ReadTextForIndex(physical string) (string, error) {
	ext := strings.ToLower(physical)
	if strings.HasSuffix(ext, ".pdf") {
		if content, err := extractPDFText(physical); err == nil && content != "" {
			return content, nil
		}
	}
	return readRawText(physical)
}

func extractPDFText(physical string) (string, error) {
	path, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", err // pdftotext not installed — skip silently
	}
	var buf bytes.Buffer
	cmd := exec.Command(path, "-l", "20", physical, "-") // first 20 pages → stdout
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	content := strings.ToValidUTF8(buf.String(), "�")
	if len(content) > MaxIndexedTextBytes {
		content = content[:MaxIndexedTextBytes]
	}
	return content, nil
}

func readRawText(physical string) (string, error) {
	file, err := os.Open(physical)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()
	content, err := io.ReadAll(io.LimitReader(file, MaxIndexedTextBytes))
	if err != nil {
		return "", err
	}
	return strings.ToValidUTF8(string(content), "�"), nil
}
