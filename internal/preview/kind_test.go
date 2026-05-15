package preview

import "testing"

func TestKindForNameExtendedPreviewTypes(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"photo.CR3":      "raw",
		"capture.dng":    "raw",
		"sheet.xlsx":     "office",
		"slides.pptx":    "office",
		"document.odt":   "office",
		"scene.glb":      "3d",
		"mesh.STL":       "3d",
		"notes.markdown": "markdown",
		"archive.tar.gz": "",
		"no-extension":   "",
	}

	for name, want := range tests {
		name, want := name, want
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := KindForName(name); got != want {
				t.Fatalf("KindForName(%q) = %q, want %q", name, got, want)
			}
		})
	}
}
