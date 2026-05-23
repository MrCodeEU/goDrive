package preview

import (
	"path/filepath"
	"strings"
)

func KindForName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif", ".avif", ".gif", ".tif", ".tiff", ".bmp", ".svg":
		return "image"
	case ".raw", ".dng", ".cr2", ".cr3", ".nef", ".arw", ".raf", ".rw2", ".orf", ".pef", ".srw":
		return "raw"
	case ".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi":
		return "video"
	case ".txt", ".log", ".csv", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini",
		".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".svelte", ".vue",
		".rs", ".java", ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".cs",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".css", ".scss", ".less", ".html", ".htm", ".sql",
		".env", ".gitignore", ".tf", ".rb", ".php", ".swift",
		".kt", ".dart", ".lua", ".r", ".jl", ".ex", ".exs":
		return "text"
	case ".md", ".markdown":
		return "markdown"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".odt", ".rtf", ".xls", ".xlsx", ".ods", ".ppt", ".pptx", ".odp":
		return "office"
	case ".glb", ".gltf", ".stl", ".obj", ".ply", ".3mf", ".step", ".stp":
		return "3d"
	default:
		return ""
	}
}
