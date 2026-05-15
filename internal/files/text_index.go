package files

import (
	"io"
	"os"
	"strings"
)

const MaxIndexedTextBytes = 512 * 1024

func SupportsTextIndex(previewKind string) bool {
	return previewKind == "text" || previewKind == "markdown"
}

func ReadTextForIndex(physical string) (string, error) {
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
	return strings.ToValidUTF8(string(content), "\uFFFD"), nil
}
