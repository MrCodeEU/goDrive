//go:build ignore

// Gen_testdata populates var/data/admin/ with a variety of test files.
// Usage: go run scripts/gen_testdata.go
package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const root = "var/data/admin"

func main() {
	dirs := []string{
		"Documents/Notes",
		"Documents/Reports",
		"Code/Go",
		"Code/Web",
		"Models/Architecture",
		"Models/Mechanical",
		"Photos/2024",
		"Photos/2025",
		"Archive",
	}
	for _, d := range dirs {
		must(os.MkdirAll(filepath.Join(root, d), 0o750))
	}

	rng := rand.New(rand.NewSource(42))

	// Text files
	writeTxt(rng, "Documents/Notes/meeting-notes.txt", 800)
	writeTxt(rng, "Documents/Notes/todo.txt", 200)
	writeTxt(rng, "Documents/Reports/annual-summary.txt", 2000)
	writeTxt(rng, "Archive/old-log.txt", 5000)

	// Markdown files
	writeMd("Documents/Notes/readme.md", "goDrive Notes", []string{
		"## Overview\nSelf-hosted file manager for families.",
		"## Features\n- Web UI\n- Mobile apps (iOS + Android)\n- TUS uploads\n- Full-text search\n- WebDAV mount",
		"## Quick Start\n```bash\nmake run\n# then open http://localhost:8121\n```",
		"## Backup Guide\n| Path | Purpose | Backup |\n|------|---------|--------|\n| `/appdata/godrive.db` | Database | **Required** |\n| `/appdata/trash/` | Trash | **Required** |\n| `/appdata/previews/` | Cache | Rebuildable |\n",
	})
	writeMd("Documents/Reports/q4-report.md", "Q4 2024 Report", []string{
		"## Executive Summary\nStorage utilisation increased 23% this quarter with 12,847 new files indexed.",
		"## File Distribution\n- Images: 8,432 (65.6%)\n- Documents: 2,104 (16.3%)\n- Videos: 893 (6.9%)\n- Other: 1,418 (11.0%)",
		"## Performance\nAverage thumbnail generation: 0.8s per image\nFull reindex duration: 4m 32s for 400k files",
		"## Action Items\n- [ ] Upgrade preview worker count\n- [ ] Enable PDF full-text search\n- [ ] Set up WebDAV mount on NAS",
	})
	writeMd("Code/Go/design.md", "Architecture Notes", []string{
		"## Source of Truth\nThe filesystem is the source of truth. SQLite stores rebuildable metadata only.",
		"## Key Invariants\n- Never trust the database for file existence — always `os.Stat`\n- Trash moves files physically, DB records the original path\n- TUS staging uses atomic rename on finalize",
		"## Performance Notes\nSQLite WAL mode with 8 connections. Per-request auth via a single JOIN (sessions + users).",
	})

	// JSON / YAML / TOML / code files
	writeFile("Code/Go/config.json", []byte(`{
  "server": { "addr": "0.0.0.0:8121", "session_ttl": "720h" },
  "storage": { "data_root": "/data", "trash_dir": "/appdata/trash" },
  "features": { "watcher": true, "reconcile_interval": "24h", "preview_timeout": "45s" }
}
`))
	writeFile("Code/Web/package.json", []byte(`{
  "name": "godrive-web",
  "version": "0.1.0",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "check": "svelte-check"
  },
  "dependencies": {
    "svelte": "^5.0.0",
    "codemirror": "^6.0.0"
  }
}
`))
	writeFile("Code/Go/config.yaml", []byte(`server:
  addr: "0.0.0.0:8121"
  session_ttl: 720h
  cookie_secure: true

storage:
  data_root: /data/users
  trash_dir: /appdata/trash
  preview_dir: /appdata/previews
  upload_dir: /appdata/uploads

features:
  watcher: true
  reconcile_interval: 24h
  preview_timeout: 45s
  preview_workers: 0
`))
	writeFile("Code/Go/deploy.toml", []byte(`[server]
addr = "0.0.0.0:8121"
session_ttl = "720h"
cookie_secure = true

[storage]
data_root = "/data/users"
trash_dir = "/appdata/trash"

[features]
watcher = true
reconcile_interval = "24h"
`))

	// Minimal PDF files
	writePDF("Documents/Reports/invoice-2024.pdf", "Invoice #2024-001", "Total: €1,234.56\nDue: 2024-12-31")
	writePDF("Documents/Reports/contract.pdf", "Service Agreement", "This agreement governs the use of goDrive for family file storage.")
	writePDF("Archive/spec-v1.pdf", "goDrive Specification v1", "Self-hosted file management system for small family setups.")

	// STL files — simple 3D shapes
	writeSTL("Models/Mechanical/cube.stl", makeCubeTriangles())
	writeSTL("Models/Mechanical/pyramid.stl", makePyramidTriangles())
	writeSTL("Models/Architecture/bracket.stl", makeBracketTriangles())

	// OBJ files
	writeOBJ("Models/Architecture/room.obj", makeRoomOBJ())
	writeOBJ("Models/Mechanical/bolt.obj", makeCylinderOBJ(16))

	// PLY file (point cloud)
	writePLY("Models/Architecture/scan.ply", 500, rng)

	// 3MF file
	write3MF("Models/Mechanical/widget.3mf")

	fmt.Printf("✓ Test data written to %s\n", root)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func writeFile(rel string, data []byte) {
	p := filepath.Join(root, rel)
	must(os.MkdirAll(filepath.Dir(p), 0o750))
	must(os.WriteFile(p, data, 0o640))
	fmt.Printf("  wrote %s (%d B)\n", rel, len(data))
}

var loremWords = strings.Fields("lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat duis aute irure dolor in reprehenderit voluptate velit esse cillum dolore eu fugiat nulla pariatur")

func writeTxt(rng *rand.Rand, rel string, words int) {
	var b strings.Builder
	b.WriteString("Created: " + time.Now().Format("2006-01-02") + "\n\n")
	for i := 0; i < words; i++ {
		if i > 0 && i%15 == 0 {
			b.WriteString("\n\n")
		} else if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(loremWords[rng.Intn(len(loremWords))])
	}
	b.WriteString("\n")
	writeFile(rel, []byte(b.String()))
}

func writeMd(rel, title string, sections []string) {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	for _, s := range sections {
		b.WriteString(s + "\n\n")
	}
	writeFile(rel, []byte(b.String()))
}

// Minimal valid PDF with one text page.
func writePDF(rel, title, body string) {
	var b bytes.Buffer
	nl := func(s string) { b.WriteString(s + "\n") }
	nl("%PDF-1.4")
	nl("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj")
	nl("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj")
	page := fmt.Sprintf("<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>")
	nl("3 0 obj" + page + "endobj")
	stream := fmt.Sprintf("BT /F1 14 Tf 50 780 Td (%s) Tj 0 -30 Td /F1 11 Tf (%s) Tj ET", title, body)
	nl(fmt.Sprintf("4 0 obj<</Length %d>>stream", len(stream)))
	nl(stream)
	nl("endstream endobj")
	nl("5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj")
	xref := b.Len()
	nl("xref\n0 6")
	nl("0000000000 65535 f\n0000000009 00000 n\n0000000058 00000 n\n0000000115 00000 n\n0000000266 00000 n\n0000000360 00000 n")
	nl(fmt.Sprintf("trailer<</Size 6/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF", xref))
	writeFile(rel, b.Bytes())
}

// ── STL ──────────────────────────────────────────────────────────────────────

type vec3 [3]float32
type tri struct{ n, v1, v2, v3 vec3 }

func writeSTL(rel string, tris []tri) {
	var b bytes.Buffer
	header := make([]byte, 80)
	copy(header, "goDrive generated STL")
	b.Write(header)
	must(binary.Write(&b, binary.LittleEndian, uint32(len(tris))))
	for _, t := range tris {
		for _, v := range []vec3{t.n, t.v1, t.v2, t.v3} {
			for _, f := range v {
				must(binary.Write(&b, binary.LittleEndian, f))
			}
		}
		b.Write([]byte{0, 0}) // attribute byte count
	}
	writeFile(rel, b.Bytes())
}

func makeCubeTriangles() []tri {
	faces := [][2][3]vec3{
		{{[3]float32{-1, -1, 1}, {1, -1, 1}, {1, 1, 1}}, {[3]float32{-1, -1, 1}, {1, 1, 1}, {-1, 1, 1}}},
		{{[3]float32{-1, -1, -1}, {-1, 1, -1}, {1, 1, -1}}, {[3]float32{-1, -1, -1}, {1, 1, -1}, {1, -1, -1}}},
		{{[3]float32{-1, 1, -1}, {-1, 1, 1}, {1, 1, 1}}, {[3]float32{-1, 1, -1}, {1, 1, 1}, {1, 1, -1}}},
		{{[3]float32{-1, -1, -1}, {1, -1, -1}, {1, -1, 1}}, {[3]float32{-1, -1, -1}, {1, -1, 1}, {-1, -1, 1}}},
		{{[3]float32{1, -1, -1}, {1, 1, -1}, {1, 1, 1}}, {[3]float32{1, -1, -1}, {1, 1, 1}, {1, -1, 1}}},
		{{[3]float32{-1, -1, -1}, {-1, -1, 1}, {-1, 1, 1}}, {[3]float32{-1, -1, -1}, {-1, 1, 1}, {-1, 1, -1}}},
	}
	normals := []vec3{{0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}, {1, 0, 0}, {-1, 0, 0}}
	var out []tri
	for i, f := range faces {
		out = append(out, tri{normals[i], f[0][0], f[0][1], f[0][2]}, tri{normals[i], f[1][0], f[1][1], f[1][2]})
	}
	return out
}

func makePyramidTriangles() []tri {
	apex := vec3{0, 0, 1}
	base := []vec3{{-1, -1, 0}, {1, -1, 0}, {1, 1, 0}, {-1, 1, 0}}
	norm := func(a, b, c vec3) vec3 {
		ux, uy, uz := b[0]-a[0], b[1]-a[1], b[2]-a[2]
		vx, vy, vz := c[0]-a[0], c[1]-a[1], c[2]-a[2]
		return vec3{uy*vz - uz*vy, uz*vx - ux*vz, ux*vy - uy*vx}
	}
	var out []tri
	for i := 0; i < 4; i++ {
		b1, b2 := base[i], base[(i+1)%4]
		n := norm(apex, b1, b2)
		out = append(out, tri{n, apex, b1, b2})
	}
	// base
	out = append(out,
		tri{vec3{0, 0, -1}, base[0], base[2], base[1]},
		tri{vec3{0, 0, -1}, base[0], base[3], base[2]},
	)
	return out
}

func makeBracketTriangles() []tri {
	// L-bracket: two rectangular boxes joined
	return append(makeBoxTris(0, 0, 0, 1, 0.1, 2), makeBoxTris(0, 0, 0, 0.1, 1, 0.5)...)
}

func makeBoxTris(x, y, z, w, d, h float32) []tri {
	x1, y1, z1 := x, y, z
	x2, y2, z2 := x+w, y+d, z+h
	faces := []struct {
		n  vec3
		vs [4]vec3
	}{
		{vec3{0, 0, 1}, [4]vec3{{x1, y1, z2}, {x2, y1, z2}, {x2, y2, z2}, {x1, y2, z2}}},
		{vec3{0, 0, -1}, [4]vec3{{x1, y2, z1}, {x2, y2, z1}, {x2, y1, z1}, {x1, y1, z1}}},
		{vec3{0, 1, 0}, [4]vec3{{x1, y2, z1}, {x1, y2, z2}, {x2, y2, z2}, {x2, y2, z1}}},
		{vec3{0, -1, 0}, [4]vec3{{x2, y1, z1}, {x2, y1, z2}, {x1, y1, z2}, {x1, y1, z1}}},
		{vec3{1, 0, 0}, [4]vec3{{x2, y1, z1}, {x2, y2, z1}, {x2, y2, z2}, {x2, y1, z2}}},
		{vec3{-1, 0, 0}, [4]vec3{{x1, y1, z2}, {x1, y2, z2}, {x1, y2, z1}, {x1, y1, z1}}},
	}
	var out []tri
	for _, f := range faces {
		out = append(out, tri{f.n, f.vs[0], f.vs[1], f.vs[2]}, tri{f.n, f.vs[0], f.vs[2], f.vs[3]})
	}
	return out
}

// ── OBJ ──────────────────────────────────────────────────────────────────────

func writeOBJ(rel, content string) { writeFile(rel, []byte(content)) }

func makeRoomOBJ() string {
	var b strings.Builder
	b.WriteString("# Room mesh\no Room\n")
	verts := [][3]float64{{0, 0, 0}, {5, 0, 0}, {5, 0, 4}, {0, 0, 4}, {0, 3, 0}, {5, 3, 0}, {5, 3, 4}, {0, 3, 4}}
	for _, v := range verts {
		fmt.Fprintf(&b, "v %.3f %.3f %.3f\n", v[0], v[1], v[2])
	}
	b.WriteString("# Floor\nf 1 2 3 4\n# Ceiling\nf 5 8 7 6\n# Walls\nf 1 5 6 2\nf 2 6 7 3\nf 3 7 8 4\nf 4 8 5 1\n")
	return b.String()
}

func makeCylinderOBJ(segments int) string {
	var b strings.Builder
	b.WriteString("# Cylinder mesh\no Cylinder\n")
	r := 0.5
	for i := 0; i < segments; i++ {
		a := 2 * math.Pi * float64(i) / float64(segments)
		fmt.Fprintf(&b, "v %.4f %.4f 0.0\n", r*math.Cos(a), r*math.Sin(a))
	}
	for i := 0; i < segments; i++ {
		a := 2 * math.Pi * float64(i) / float64(segments)
		fmt.Fprintf(&b, "v %.4f %.4f 1.0\n", r*math.Cos(a), r*math.Sin(a))
	}
	fmt.Fprintf(&b, "v 0.0 0.0 0.0\nv 0.0 0.0 1.0\n")
	bot, top := segments*2+1, segments*2+2
	for i := 1; i <= segments; i++ {
		n := i%segments + 1
		fmt.Fprintf(&b, "f %d %d %d %d\n", i, n, n+segments, i+segments)
		fmt.Fprintf(&b, "f %d %d %d\n", bot, n, i)
		fmt.Fprintf(&b, "f %d %d %d\n", top, i+segments, n+segments)
	}
	return b.String()
}

// ── PLY ──────────────────────────────────────────────────────────────────────

func writePLY(rel string, n int, rng *rand.Rand) {
	var b strings.Builder
	fmt.Fprintf(&b, "ply\nformat ascii 1.0\nelement vertex %d\nproperty float x\nproperty float y\nproperty float z\nproperty uchar red\nproperty uchar green\nproperty uchar blue\nend_header\n", n)
	for i := 0; i < n; i++ {
		x := rng.Float32()*10 - 5
		y := rng.Float32()*10 - 5
		z := rng.Float32() * 3
		r := uint8(rng.Intn(256))
		g := uint8(rng.Intn(256))
		bl := uint8(rng.Intn(256))
		fmt.Fprintf(&b, "%.4f %.4f %.4f %d %d %d\n", x, y, z, r, g, bl)
	}
	writeFile(rel, []byte(b.String()))
}

// ── 3MF ──────────────────────────────────────────────────────────────────────

func write3MF(rel string) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	addEntry := func(name, content string) {
		w, err := zw.Create(name)
		must(err)
		_, err = w.Write([]byte(content))
		must(err)
	}

	addEntry("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="model" ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"/>
</Types>`)

	addEntry("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Target="/3D/model.model" Id="rel0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
</Relationships>`)

	addEntry("3D/model.model", `<?xml version="1.0" encoding="UTF-8"?>
<model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
  <resources>
    <object id="1" type="model">
      <mesh>
        <vertices>
          <vertex x="0" y="0" z="0"/>
          <vertex x="10" y="0" z="0"/>
          <vertex x="10" y="10" z="0"/>
          <vertex x="0" y="10" z="0"/>
          <vertex x="0" y="0" z="10"/>
          <vertex x="10" y="0" z="10"/>
          <vertex x="10" y="10" z="10"/>
          <vertex x="0" y="10" z="10"/>
        </vertices>
        <triangles>
          <triangle v1="0" v2="1" v3="2"/><triangle v1="0" v2="2" v3="3"/>
          <triangle v1="4" v2="6" v3="5"/><triangle v1="4" v2="7" v3="6"/>
          <triangle v1="0" v2="4" v3="5"/><triangle v1="0" v2="5" v3="1"/>
          <triangle v1="1" v2="5" v3="6"/><triangle v1="1" v2="6" v3="2"/>
          <triangle v1="2" v2="6" v3="7"/><triangle v1="2" v2="7" v3="3"/>
          <triangle v1="3" v2="7" v3="4"/><triangle v1="3" v2="4" v3="0"/>
        </triangles>
      </mesh>
    </object>
  </resources>
  <build><item objectid="1"/></build>
</model>`)

	must(zw.Close())
	writeFile(rel, buf.Bytes())
}
