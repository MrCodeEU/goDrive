//go:build ignore

// gen_testdata populates var/data/admin/ with realistic test files.
// Generates ~260 files for stress-testing indexing, FTS, and pagination.
//
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

var fileCount int
var rng = rand.New(rand.NewSource(42))

func main() {
	for _, d := range []string{
		"Documents/Personal/Letters", "Documents/Personal/Notes", "Documents/Personal/Forms",
		"Documents/Work/Projects/Alpha", "Documents/Work/Projects/Beta", "Documents/Work/Projects/Gamma",
		"Documents/Work/Reports/2023", "Documents/Work/Reports/2024", "Documents/Work/Reports/2025",
		"Documents/Work/Presentations",
		"Documents/Finance/2022", "Documents/Finance/2023", "Documents/Finance/2024",
		"Photos/2022/Summer", "Photos/2022/Winter",
		"Photos/2023/Vacation", "Photos/2023/Christmas",
		"Photos/2024/Family", "Photos/2024/Travel",
		"Photos/2025",
		"Code/Go", "Code/Python", "Code/Web", "Code/Shell",
		"Models/3DPrinting/Parts", "Models/3DPrinting/Assemblies",
		"Models/Architecture", "Models/Mechanical",
		"Archive/2020", "Archive/2021", "Archive/2022",
		"Shared/Recipes", "Shared/Plans",
		"Media/Podcasts", "Media/Playlists",
		"Large-Folder",
	} {
		must(os.MkdirAll(filepath.Join(root, d), 0o750))
	}

	genPersonalNotes()
	genLetters()
	genForms()
	genWorkDocs()
	genFinance()
	genCodeFiles()
	genPhotos()
	genModels()
	genArchive()
	genShared()
	genMedia()
	genLargeFolder() // 120 files for pagination stress

	fmt.Printf("\n✓ %d files written to %s\n", fileCount, root)
}

// ── generators ────────────────────────────────────────────────────────────────

func genPersonalNotes() {
	writeMd("Documents/Personal/Notes/meeting-notes.md", "Meeting Notes 2024-Q4", []string{
		"## Attendees\nAlice, Bob, Carol, David — weekly sync",
		"## Topics\n1. Q4 roadmap review — mobile app launch on track\n2. Hiring: 2 engineers by end of Q4\n3. Infrastructure: migrate to arm64 runners to cut CI costs 40%",
		"## Action Items\n- [ ] Alice: finalize roadmap doc by Friday\n- [ ] Bob: schedule engineering interviews\n- [ ] Carol: update budget forecast",
	})
	writeMd("Documents/Personal/Notes/book-list.md", "Reading List", []string{
		"## Currently Reading\n- *Designing Data-Intensive Applications* — Martin Kleppmann\n- *The Pragmatic Programmer* — Thomas & Hunt",
		"## Want to Read\n- *Staff Engineer* — Will Larson\n- *Database Internals* — Alex Petrov\n- *The Manager's Path* — Camille Fournier",
		"## Finished ★★★★★\n- *The Phoenix Project* — Kim, Behr, Spafford\n- *Accelerate* — Forsgren, Humble, Kim",
	})
	writeMd("Documents/Personal/Notes/vacation-plan.md", "Vacation Planning 2025", []string{
		"## Options\n1. **Japan** April — cherry blossom season, 14 days, ~€4000pp\n2. **Portugal** September — shoulder season, 12 days, ~€2500pp\n3. **Iceland** August — midnight sun, 10 days, ~€3500pp",
		"## Budget\nFlights: €600-1400pp\nAccommodation: €80-150/night\nFood & activities: €80-120/day",
		"## Checklist\n- [ ] Book flights 3-4 months ahead\n- [ ] Travel insurance\n- [ ] Accommodation 2-3 months ahead\n- [ ] Airport transfer",
	})
	writeTxt("Documents/Personal/Notes/shopping-list.txt",
		"Groceries:\n- Milk, eggs, bread\n- Spinach, tomatoes, bell peppers, broccoli\n- Chicken thighs, salmon fillet\n- Greek yogurt, cheddar, parmesan\n- Almonds, dark chocolate (85%)\n- Lemons, garlic, fresh ginger\n\nHousehold:\n- Laundry detergent (unscented)\n- Dish soap\n- Paper towels (bulk)\n- LED bulbs 2700K E27\n\nElectronics:\n- USB-C cable 2m braided\n- SD card 256GB UHS-II for camera\n")
	writeTxt("Documents/Personal/Notes/ideas.txt",
		"Ideas & Brainstorm\n==================\n\n2024-11-15\n- Home automation: temperature sensors in every room, automated heating zones\n- Smart irrigation for the garden based on rainfall data\n- Offline-first mobile app for recipe management (SQLite + sync)\n- 3D printed cable management clips for home office\n\n2024-10-03\n- Build a NAS with ZFS mirror for family photos\n- Set up automated offsite backup to Backblaze B2\n- Create a shared family calendar that syncs with goDrive\n")
}

func genLetters() {
	bodies := []string{
		"Dear Friend,\n\nI hope this letter finds you well. It has been too long since we last caught up properly.\n\nLife here has been busy but rewarding. We recently moved to a new apartment with a beautiful view — longer commute but worth the extra space.\n\nI would love to visit in spring when the weather improves. Let me know if that works.\n\nWarm regards",
		"To Whom It May Concern,\n\nI am writing regarding the community garden program mentioned at the last neighbourhood meeting.\n\nI am particularly interested in the plot allocation process, annual fees, and rules about what may be grown. Please forward any relevant documentation.\n\nSincerely",
		"Dear Landlord,\n\nI am writing regarding the ongoing heating system fault in apartment 4B.\n\nDespite reporting the issue three weeks ago, no repair has been scheduled. The radiator in the bedroom is non-functional during winter — this requires urgent attention.\n\nI request a qualified technician within five business days.\n\nRegards",
		"Dear HR Department,\n\nThank you for the offer letter dated November 18. I am pleased to accept the position of Senior Software Engineer.\n\nI confirm my start date as December 2nd and will bring the signed contract on my first day.\n\nI look forward to joining the team.\n\nBest regards",
	}
	for i, b := range bodies {
		writeTxt(fmt.Sprintf("Documents/Personal/Letters/letter-%02d.txt", i+1), b)
	}
}

func genForms() {
	writeTxt("Documents/Personal/Forms/tax-2024.txt",
		"Tax Year: 2024\nFiling Status: Single / Joint [circle one]\nEmployment Income: see W-2 / P60\nFreelance Income: €4,200\nCapital Gains: €1,850 (shares sold)\nDeductible: home office 12sqm, professional subscriptions, hardware\n\nDue Date: 2025-07-31\nEstimated Refund: TBD after full calculation\n")
	writeTxt("Documents/Personal/Forms/insurance-policy.txt",
		"Policy: HH-2024-789012\nType: Household contents + Personal liability\nInsurer: Allianz SE\nCoverage: €60,000 contents / €5,000,000 liability\nPremium: €264/year (paid annually January)\nRenewal: 2025-01-01\n\nIncluded: fire, water damage, theft, accidental breakage rider\nExcluded: flooding (separate flood insurance required)\n")
	writeTxt("Documents/Personal/Forms/passport-checklist.txt",
		"Passport Renewal Checklist\nExpiry: 2025-08-15 — renew from 2025-02-15\n\n[ ] Current passport (original)\n[ ] 2× biometric photos 35×45mm\n[ ] Completed application form (downloaded from govt website)\n[ ] €70 fee (credit card accepted)\n[ ] Proof of citizenship if first renewal\n\nProcessing: 4-6 weeks standard, 2-3 weeks express (+€30)\nCollection: in person or postal (tracked)\n")
}

func genWorkDocs() {
	projects := []struct {
		dir, slug, title string
		sections         []string
	}{
		{"Alpha", "api-design", "API Design Specification", []string{
			"## Authentication\nBearer token + session cookie. Argon2id password hashing.\nCSRF protection on mutating requests. Rate-limited login (10 fails/IP/5min → 15min block).",
			"## File Operations\nAll paths relative to user home root. Symlinks confined to root.\nOperations: list (paginated), mkdir, move, bulk-delete, trash, restore, download.",
			"## Upload Protocol\nTUS-compatible resumable upload at /api/tus. Staging to temp file. Atomic rename on finalize. Post-upload thumbnail warmup for image/raw/video/pdf/office.",
			"## Search\nFTS5 trigram index on filename + path. Document FTS on text/markdown/PDF content. BM25 ranking. LIKE fallback for short queries (< 3 chars).",
		}},
		{"Beta", "mobile-architecture", "Mobile App Architecture", []string{
			"## State Management\nProvider. AuthState holds API client + session. UploadQueue drives concurrent TUS workers (max 3).",
			"## iOS Background Upload\nNative URLSession background config. Bridged via MethodChannel 'godrive/background_uploads'.\nUpload state persisted in SharedPreferences. Resume on app relaunch.",
			"## Android Foreground Service\nForeground service with dataSync type. Progress notification. Reconnects to existing TUS uploads on resume.",
			"## Share Extension (iOS)\nSLComposeServiceViewController. Media copied to App Group container.\nStored as JSON in UserDefaults (suite: group.eu.mljr.godrive, key: ShareKey).\nHost app opened via ShareMedia-eu.mljr.godrive:share URL scheme.",
		}},
		{"Gamma", "search-improvements", "Search Improvements Roadmap", []string{
			"## Current\nFTS5 trigram on file_index_fts. Document FTS on document_fts.\nPDF extraction via pdftotext (first 20 pages, 512KB cap, cached lookup).",
			"## Planned Q1 2025\n- Office text extraction via libreoffice --convert-to txt\n- Search result snippets with highlighted terms\n- Recent searches stored locally\n- File type facets in search results",
			"## Planned Q2 2025\n- BM25 parameter tuning\n- Query expansion with synonyms\n- OCR for scanned PDFs (via tesseract)\n- Audio transcription for podcast notes",
		}},
		{"Alpha", "deploy-checklist", "Production Deploy Checklist", []string{
			"## Pre-deploy\n- [ ] `make check` — all tests green\n- [ ] Review env vars for breaking changes\n- [ ] Backup database and trash directory\n- [ ] Verify Docker image builds amd64 + arm64",
			"## Deploy\n- [ ] Pull latest image\n- [ ] Graceful stop existing container (wait for uploads)\n- [ ] Start new container\n- [ ] Verify /health → 200",
			"## Post-deploy\n- [ ] Run `godrive verify`\n- [ ] Check logs for startup errors\n- [ ] Test upload, download, thumbnail\n- [ ] Verify WebDAV mount from client\n- [ ] Check Tailscale connectivity",
		}},
	}
	for _, p := range projects {
		writeMd(fmt.Sprintf("Documents/Work/Projects/%s/%s.md", p.dir, p.slug), p.title, p.sections)
	}
	for _, p := range projects {
		writeTxt(fmt.Sprintf("Documents/Work/Projects/%s/notes.txt", p.dir),
			fmt.Sprintf("Notes for %s\nLast updated: %s\n\nSee %s.md for full specification.\n", p.title, time.Now().Format("2006-01-02"), p.slug))
	}

	reports := []struct{ year, slug, title string }{
		{"2024", "q1-report", "Q1 2024 Progress Report"},
		{"2024", "q2-report", "Q2 2024 Progress Report"},
		{"2024", "q3-report", "Q3 2024 Progress Report"},
		{"2025", "q1-report", "Q1 2025 Progress Report"},
		{"2023", "annual", "Annual Review 2023"},
	}
	for _, r := range reports {
		sections := []string{
			"## Summary\nFiles indexed: 412,847 (+" + fmt.Sprintf("%d", 5+rng.Intn(20)) + "% QoQ). Preview cache: 28.4 GB. Active users: 4.",
			"## Performance\nMedian list latency: 12ms. p99: 45ms. Thumbnail cache hit rate: 94%.",
			"## Issues & Fixes\n- Watcher missed events during SMB rename — fixed with reconciliation scan\n- Background upload edge case on iOS 17.x — patched in hotfix release",
		}
		writeMd(fmt.Sprintf("Documents/Work/Reports/%s/%s.md", r.year, r.slug), r.title, sections)
	}

	pres := []struct{ slug, body string }{
		{"fts-talk.txt", "Full-Text Search in SQLite\n\nSlide 1: FTS5 Overview\nFTS5 is SQLite's fifth-generation full-text search. Supports BM25 ranking, trigram tokenization, content tables, and auxiliary functions.\n\nSlide 2: Trigram Tokenization\nSplits text into overlapping 3-char sequences. Allows substring matching: 'rive' matches 'goDrive'. Index size ~3x source.\n\nSlide 3: Integration Pattern\nSELECT fi.* FROM file_index_fts JOIN file_index fi ON fi.path = file_index_fts.path WHERE file_index_fts MATCH 'query'\n\nSlide 4: Graceful Fallback\nShort queries (< 3 chars) fall back to LIKE search. Error on FTS also triggers LIKE fallback."},
		{"webdav-intro.txt", "WebDAV Integration\n\nSlide 1: Protocol\nHTTP extension (RFC 4918). Supported natively by macOS Finder, Windows Explorer, iOS Files, Cyberduck, rclone.\n\nSlide 2: Methods\nPROPFIND — list properties\nMKCOL — create directory\nCOPY/MOVE — copy or move\nLOCK/UNLOCK — exclusive access\n\nSlide 3: Implementation\ngolang.org/x/net/webdav. webdav.Dir(homeRoot) as filesystem.\nPer-user in-memory LockSystem. Basic Auth + Bearer/Cookie auth.\n\nSlide 4: Client Setup\nmacOS: Finder → Go → Connect to Server → http://host:8121/dav/\nrclone: type=webdav url=http://host:8121/dav/ user=alice pass=xxx"},
	}
	for _, p := range pres {
		writeTxt("Documents/Work/Presentations/"+p.slug, p.body)
	}
}

func genFinance() {
	invoices := []struct{ year, name, vendor, body string }{
		{"2024", "invoice-cloudflare-jan.txt", "Cloudflare Inc", "Invoice #CF-2024-001\nDate: 2024-01-01\nVendor: Cloudflare Inc\nItem: Pro Plan — January 2024\nAmount: $20.00 USD\nPayment: credit card *4242\nStatus: PAID"},
		{"2024", "invoice-github-feb.txt", "GitHub Inc", "Invoice #GH-2024-0201\nDate: 2024-02-01\nVendor: GitHub Inc\nItem: Team Plan 4 seats — Feb 2024\nAmount: $44.00 USD\nStatus: PAID"},
		{"2024", "invoice-digitalocean-mar.txt", "DigitalOcean LLC", "Invoice #DO-2024-03\nDate: 2024-03-01\nDroplet 2GB/1CPU: $12.00\nSpaces 250GB: $6.00\nTotal: $18.00 USD\nStatus: PAID"},
		{"2024", "receipt-electricity-mar.txt", "Stadtwerke GmbH", "Rechnung 2024-03-45678\nVerbrauch: 415 kWh\nGrundpreis: €9.50\nArbeitspreis 0.278€/kWh: €115.37\nMwSt 19%: €23.54\nGesamt: €148.41\nFällig: 2024-04-15\nStatus: BEZAHLT"},
		{"2024", "receipt-internet-apr.txt", "Telekom AG", "Rechnung April 2024\nGigabit DSL Flatrate: €39.99\nMwSt 19%: €7.60\nGesamt: €47.59\nAbbuchung: 2024-04-10\nStatus: BEZAHLT"},
		{"2024", "invoice-amazon-may.txt", "Amazon EU", "Order #123-456-789\nDate: 2024-05-10\nItem: Samsung 990 Pro 2TB NVMe: €94.99\nShipping: FREE\nDelivery: 2024-05-12\nStatus: DELIVERED"},
		{"2023", "invoice-netcup-annual.txt", "Netcup GmbH", "Rechnung 2023-JA-001\nVPS 2000 G9 Jahresvertrag\n12 × €7.49 = €89.88\nMwSt 19%: €17.08\nBrutto: €106.96\nBezahlt: 2023-12-01"},
		{"2023", "receipt-dentist.txt", "Zahnarztpraxis Dr. Müller", "Rechnung 2023-089\nDatum: 2023-09-15\nUntersuchung: €30.00\nProfessionelle Zahnreinigung: €90.00\n2× Röntgen: €60.00\nSumme: €180.00\nKassenbeteiligung: €30.00\nSelbstbeteiligung: €150.00\nBezahlt: 2023-09-20"},
		{"2022", "insurance-annual-2022.txt", "Allianz SE", "Police HH-2022-789012\nHausratversicherung Jahresprämie 2022\nInhaltsversicherung €50.000: €180.00\nHaftpflicht €5.000.000: €84.00\nGesamt: €264.00\nBezahlt: 2022-01-05"},
	}
	for _, inv := range invoices {
		writeTxt("Documents/Finance/"+inv.year+"/"+inv.name, inv.body)
		writePDF("Documents/Finance/"+inv.year+"/"+strings.TrimSuffix(inv.name, ".txt")+".pdf", inv.vendor, inv.body)
	}
}

func genCodeFiles() {
	files := []struct{ path, content string }{
		{"Go/main.go", "//go:build ignore\n\npackage main\n\nimport (\n\t\"context\"\n\t\"log/slog\"\n\t\"os\"\n\t\"os/signal\"\n\t\"syscall\"\n)\n\nfunc main() {\n\tlog := slog.New(slog.NewTextHandler(os.Stdout, nil))\n\tctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)\n\tdefer stop()\n\tif err := run(ctx, log); err != nil {\n\t\tlog.Error(\"fatal\", \"err\", err)\n\t\tos.Exit(1)\n\t}\n}\n"},
		{"Go/config.go", "//go:build ignore\n\npackage main\n\nimport (\n\t\"os\"\n\t\"time\"\n)\n\ntype Config struct {\n\tAddr           string\n\tDataRoot       string\n\tSessionTTL     time.Duration\n\tEnableWatcher  bool\n\tPreviewWorkers int\n}\n\nfunc loadConfig() Config {\n\treturn Config{\n\t\tAddr:          env(\"GODRIVE_ADDR\", \"127.0.0.1:8121\"),\n\t\tDataRoot:      env(\"GODRIVE_DATA_ROOT\", \"./var/data\"),\n\t\tSessionTTL:    720 * time.Hour,\n\t\tEnableWatcher: true,\n\t}\n}\n\nfunc env(k, fallback string) string {\n\tif v := os.Getenv(k); v != \"\" {\n\t\treturn v\n\t}\n\treturn fallback\n}\n"},
		{"Python/batch_rename.py", "#!/usr/bin/env python3\n\"\"\"Batch rename files using a pattern.\"\"\"\nimport argparse\nimport os\nfrom datetime import datetime\nfrom pathlib import Path\n\n\ndef rename_files(directory: str, pattern: str, dry_run: bool = True) -> None:\n    path = Path(directory)\n    files = sorted(f for f in path.iterdir() if f.is_file())\n    for idx, file in enumerate(files, start=1):\n        ext = file.suffix.lower()\n        date = datetime.now().strftime(\"%Y%m%d\")\n        new_name = pattern.format(date=date, index=idx, ext=ext, stem=file.stem)\n        if dry_run:\n            print(f\"[DRY] {file.name} -> {new_name}\")\n        else:\n            file.rename(path / new_name)\n            print(f\"Renamed: {file.name} -> {new_name}\")\n\n\nif __name__ == \"__main__\":\n    parser = argparse.ArgumentParser()\n    parser.add_argument(\"--dir\", default=\".\")\n    parser.add_argument(\"--pattern\", default=\"{stem}{ext}\")\n    parser.add_argument(\"--apply\", action=\"store_true\")\n    args = parser.parse_args()\n    rename_files(args.dir, args.pattern, dry_run=not args.apply)\n"},
		{"Python/search_api.py", "#!/usr/bin/env python3\n\"\"\"Search goDrive via API.\"\"\"\nimport json\nimport sys\nimport urllib.parse\nimport urllib.request\n\n\ndef search(server: str, token: str, query: str, limit: int = 50):\n    url = f\"{server}/api/files/search?q={urllib.parse.quote(query)}&limit={limit}\"\n    req = urllib.request.Request(url, headers={\"Authorization\": f\"Bearer {token}\"})\n    with urllib.request.urlopen(req) as resp:\n        return json.loads(resp.read())[\"entries\"]\n\n\nif __name__ == \"__main__\":\n    server = sys.argv[1] if len(sys.argv) > 1 else \"http://localhost:8121\"\n    token = sys.argv[2] if len(sys.argv) > 2 else \"\"\n    query = \" \".join(sys.argv[3:]) if len(sys.argv) > 3 else \"test\"\n    for e in search(server, token, query):\n        print(f\"{e['type']:4}  {e['size']:12,}  {e['path']}\")\n"},
		{"Web/api.ts", "export type FileEntry = {\n  name: string;\n  path: string;\n  type: 'file' | 'dir';\n  size: number;\n  modified_at: string;\n  mime_type?: string;\n  preview_kind?: string;\n};\n\nexport function formatBytes(n: number): string {\n  if (n < 1024) return n + ' B';\n  const units = ['KB', 'MB', 'GB', 'TB'];\n  let s = n / 1024;\n  for (const u of units) {\n    if (s < 1024) return s.toFixed(s >= 10 ? 0 : 1) + ' ' + u;\n    s /= 1024;\n  }\n  return s.toFixed(1) + ' PB';\n}\n\nexport const thumbnailURL = (path: string, size = 256) =>\n  `/api/files/thumbnail?path=${encodeURIComponent(path)}&size=${size}`;\n"},
		{"Web/tokens.css", ":root {\n  --bg-0: #0b0b0e;\n  --bg-1: #12121a;\n  --bg-2: #1a1a26;\n  --bg-3: #22223a;\n  --accent: #f97316;\n  --accent-dim: rgba(249,115,22,.1);\n  --text-1: #eeeef8;\n  --text-2: #8888b0;\n  --text-3: #44445a;\n  --radius-sm: 6px;\n  --radius-md: 10px;\n  --radius-lg: 16px;\n  font-family: 'Plus Jakarta Sans', ui-sans-serif, sans-serif;\n}\n"},
		{"Shell/backup.sh", "#!/bin/bash\nset -euo pipefail\nAPPDATA=\"${GODRIVE_APPDATA:-/var/appdata/godrive}\"\nDEST=\"${1:-/mnt/backup/godrive}\"\nDATE=$(date +%Y%m%d-%H%M%S)\necho \"Backup started: $DATE\"\nmkdir -p \"$DEST/$DATE\"\ncp \"$APPDATA/godrive.sqlite\" \"$DEST/$DATE/\"\necho \"  ✓ database\"\nrsync -a --delete \"$APPDATA/trash/\" \"$DEST/$DATE/trash/\"\necho \"  ✓ trash\"\nln -sfn \"$DEST/$DATE\" \"$DEST/latest\"\necho \"Backup complete: $(du -sh $DEST/$DATE | cut -f1)\"\n"},
		{"Go/config.yaml", "server:\n  addr: \"0.0.0.0:8121\"\n  session_ttl: 720h\n  cookie_secure: true\n\nstorage:\n  data_root: /data/users\n  trash_dir: /appdata/trash\n  preview_dir: /appdata/previews\n  upload_dir: /appdata/uploads\n\nfeatures:\n  watcher: true\n  reconcile_interval: 24h\n  preview_timeout: 45s\n  preview_workers: 0\n  max_upload_bytes: 0\n"},
		{"Go/deploy.toml", "[server]\naddr = \"0.0.0.0:8121\"\nsession_ttl = \"720h\"\ncookie_secure = true\n\n[storage]\ndata_root = \"/data/users\"\ntrash_dir = \"/appdata/trash\"\npreview_dir = \"/appdata/previews\"\n\n[features]\nwatcher = true\nreconcile_interval = \"24h\"\n"},
		{"Shell/reindex.sh", "#!/bin/bash\n# Trigger a full reindex via the goDrive CLI.\nset -euo pipefail\nCONTAINER=\"${1:-godrive}\"\necho \"Starting full reindex in container: $CONTAINER\"\ndocker exec \"$CONTAINER\" godrive reindex\necho \"Done.\"\n"},
	}
	for _, f := range files {
		writeFile("Code/"+f.path, []byte(f.content))
	}
}

func genPhotos() {
	dirs := []string{"2022/Summer", "2022/Winter", "2023/Vacation", "2023/Christmas", "2024/Family", "2024/Travel", "2025"}
	cameras := []string{"iPhone 15 Pro", "Canon EOS R6", "Sony A7IV", "Fujifilm X-T5"}
	for _, dir := range dirs {
		n := 5 + rng.Intn(10)
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("IMG_%04d.txt", 1000+rng.Intn(9000))
			body := fmt.Sprintf("Album: %s\nCamera: %s\nISO: %d\nAperture: f/%.1f\nShutter: 1/%d\nFocal: %dmm\nDate: 2024-%02d-%02d\n",
				dir, cameras[rng.Intn(len(cameras))],
				100*rng.Intn(32), float64(rng.Intn(14))/2.0+1.8, 30+rng.Intn(2000),
				14+rng.Intn(200), 1+rng.Intn(11), 1+rng.Intn(27))
			writeTxt("Photos/"+dir+"/"+name, body)
		}
	}
}

func genModels() {
	// STL files
	writeSTL("Models/3DPrinting/Parts/cube-20mm.stl", boxTris(0, 0, 0, 20, 20, 20))
	writeSTL("Models/3DPrinting/Parts/cylinder-r10h30.stl", cylTris(24, 10, 30))
	writeSTL("Models/3DPrinting/Parts/sphere-r15.stl", sphereTris(12, 24, 15))
	writeSTL("Models/3DPrinting/Parts/hex-standoff.stl", cylTris(6, 3, 10))
	writeSTL("Models/Mechanical/enclosure-base.stl", append(boxTris(0, 0, 0, 80, 60, 40), boxTris(2, 2, 2, 76, 56, 38)...))
	writeSTL("Models/Mechanical/spur-gear.stl", cylTris(32, 25, 8))
	writeSTL("Models/Architecture/column.stl", cylTris(16, 5, 40))
	// OBJ
	writeFile("Models/Architecture/room.obj", []byte("# Room\no Room\nv 0 0 0\nv 6 0 0\nv 6 0 5\nv 0 0 5\nv 0 3 0\nv 6 3 0\nv 6 3 5\nv 0 3 5\n# floor ceiling walls\nf 1 2 3 4\nf 5 8 7 6\nf 1 5 6 2\nf 2 6 7 3\nf 3 7 8 4\nf 4 8 5 1\n"))
	writeFile("Models/Mechanical/shaft.obj", []byte(cylinderOBJ(20, 0.5, 8.0)))
	// PLY
	for i := 0; i < 5; i++ {
		writePLY(fmt.Sprintf("Models/3DPrinting/Parts/scan-%02d.ply", i+1), 300+rng.Intn(700))
	}
	// 3MF
	write3MF("Models/3DPrinting/Assemblies/enclosure.3mf")
	write3MF("Models/3DPrinting/Assemblies/bracket-template.3mf")
}

func genArchive() {
	snippets := []string{
		"Archive entry. Records from early project phase migrated to current system.\nOriginal format: CSV. Retained per 7-year data retention policy.",
		"Quarterly backup record. All services operational at backup time.\nDatabase: 2.3 GB. Files indexed: 187,432. Duration: 4m 12s.",
		"System config snapshot. nginx.conf + docker-compose.yml saved before migration.\nDo not restore without compatibility review.",
		"User data export per GDPR Art. 20. Contains: profile, file metadata, 90-day activity log.",
	}
	for i := 0; i < 20; i++ {
		year := []string{"2020", "2021", "2022"}[i%3]
		writeTxt(fmt.Sprintf("Archive/%s/archive-%02d.txt", year, i+1), snippets[i%len(snippets)])
	}
}

func genShared() {
	recipes := []struct {
		slug, title string
		sections    []string
	}{
		{"pizza-sourdough.md", "Sourdough Pizza", []string{
			"## Dough (3 pizzas)\n- 500g bread flour (00)\n- 350g water (70% hydration)\n- 100g active sourdough starter\n- 10g salt\n\nMix, autolyse 30min, fold 4× at 30min intervals. Cold ferment 24-48h.",
			"## Baking\n280°C on preheated stone, 6-8 minutes. Toppings: San Marzano, torn mozzarella, fresh basil after bake.",
		}},
		{"ramen-tonkotsu.md", "Tonkotsu Ramen", []string{
			"## Broth (10-12 hours)\n1kg pork trotters + 500g neck bones. Blanch 10min, drain. Fresh water, simmer uncovered.\nStrain, season with soy, mirin, sake.",
			"## Toppings\nChashu pork belly. Marinated soft eggs (6min, overnight marinade).\nNori, bamboo shoots, spring onion, black garlic oil.",
		}},
		{"croissants.md", "Butter Croissants", []string{
			"## Détrempe (Day 1)\n500g flour, 300g warm milk, 10g instant yeast, 50g sugar, 10g salt, 30g butter.\nMix until just combined. Refrigerate overnight.",
			"## Lamination (Day 2)\n300g cold butter beaten to 20cm square. Encase in dough, 3 letter folds.\nRefrigerate 30min between folds. Cut triangles, roll, proof 2-3h, bake 200°C 18min.",
		}},
		{"sourdough-loaf.md", "Sourdough Country Loaf", []string{
			"## Levain (night before)\n20g starter + 100g flour + 100g water. Leave 8-12h until doubled.",
			"## Dough (75% hydration)\n450g bread flour, 50g wholemeal, 375g water, 100g levain, 10g salt.\n4 stretch-and-fold sets at 30min intervals. Shape, banneton, overnight refrigerator proof.",
			"## Bake\nPreheated Dutch oven 250°C. Covered 20min, uncovered 25min until deep brown crust.",
		}},
	}
	for _, r := range recipes {
		writeMd("Shared/Recipes/"+r.slug, r.title, r.sections)
	}

	writeTxt("Shared/Plans/summer-2025.txt", "Summer 2025\n\nJune: Garden party for Grandma's 70th\n- ~30 guests, self-catered\n- Music: cousin Max on acoustic guitar\n\nJuly 12-26: Portugal vacation (Algarve)\n- Villa rental 4 bedrooms, booked\n- Flights: EasyJet, car: Hertz\n- Activities: beach, Lagos day trip, cave boat tour\n\nAugust: Kids summer camps\n- Week 1: sports camp\n- Week 2: coding camp (ages 10-14)\n- Week 3: art camp\n")
	writeTxt("Shared/Plans/christmas-2024.txt", "Christmas 2024\n\nDec 23: Arrive at parents'\nDec 24: Fondue tradition, present opening at midnight\nDec 25: Morning walk, afternoon board games, evening with in-laws\n\nGifts:\n- Dad: woodworking book + new chisels set\n- Mom: spa voucher + cashmere scarf\n- Brother: camera lens (split with sister)\n- Sister: cooking class\n- Kids: LEGO Technic, art supplies, science kit\n")
}

func genMedia() {
	writeTxt("Media/Podcasts/software-engineering-daily.txt", "Software Engineering Daily — Notes\n\n'SQLite in Production'\n- WAL mode enables concurrent reads\n- Cloudflare D1, Turso: SQLite at the edge\n- Don't dismiss SQLite for server workloads\n\n'WebAssembly Beyond the Browser'\n- WASM in server runtimes (Wasmtime, WasmEdge)\n- WASI: system call interface\n- Plugin systems and sandboxed execution\n\n'Distributed Tracing with OpenTelemetry'\n- Spans, traces, context propagation\n- Sampling strategies for high-throughput systems\n")
	writeTxt("Media/Podcasts/hardcore-history-ww1.txt", "Hardcore History: Blueprint for Armageddon\n\nPart I: The July Crisis\nAssassination of Franz Ferdinand, June 28 1914. Austria-Hungary ultimatum.\nMobilization plans locked all powers into escalation — stopping the war would have been harder than starting it.\n\nPart II: The Guns of August\nSchlieffen Plan: knockout blow against France first. Belgium neutrality violated — Britain enters.\nTrenches established by December 1914.\n")
	writeTxt("Media/Playlists/focus-work.m3u", "#EXTM3U\n#PLAYLIST:Focus Work\n#EXTINF:198,Brian Eno - Music For Airports 1/1\n#EXTINF:223,Nils Frahm - Says\n#EXTINF:312,Max Richter - On The Nature Of Daylight\n#EXTINF:445,Johann Johannsson - The Sun's Gone Dim\n#EXTINF:267,Ólafur Arnalds - Near Light\n")
	writeTxt("Media/Playlists/weekend-cooking.m3u", "#EXTM3U\n#PLAYLIST:Weekend Cooking\n#EXTINF:243,The Beatles - Here Comes The Sun\n#EXTINF:198,Jack Johnson - Banana Pancakes\n#EXTINF:274,Norah Jones - Come Away With Me\n#EXTINF:223,Ben Harper - Steal My Kisses\n#EXTINF:254,Feist - 1234\n")
}

func genLargeFolder() {
	topics := []string{
		"database", "network", "security", "performance", "api", "cache", "queue",
		"auth", "storage", "search", "index", "webhook", "session", "config", "deploy",
		"monitor", "log", "trace", "metric", "alert",
	}
	sentences := [][]string{
		{"SQLite WAL mode enables concurrent reads.", "FTS5 trigram indexes support substring matching.", "B-tree indexes support range queries efficiently.", "EXPLAIN QUERY PLAN reveals how SQLite executes queries.", "Foreign keys enforce referential integrity."},
		{"TCP provides reliable ordered delivery.", "HTTP/2 multiplexes requests over one connection.", "TLS 1.3 reduces handshake round-trips.", "WebSockets enable full-duplex communication.", "DNS TTL controls client-side caching duration."},
		{"Argon2id is the recommended password hash for 2024.", "CSRF tokens prevent cross-site request forgery.", "Rate limiting protects login endpoints.", "Content Security Policy reduces XSS surface.", "Use constant-time comparison for secrets."},
		{"Caching reduces database round-trips.", "Profile before optimizing.", "Connection pooling amortizes connection cost.", "Batch operations reduce per-item overhead.", "Goroutines allow high concurrency cheaply."},
		{"REST: GET reads, POST creates, PATCH updates.", "Pagination prevents memory exhaustion.", "API versioning allows breaking changes.", "Idempotent endpoints are safe to retry.", "Bearer tokens are the API auth standard."},
		{"LRU eviction removes least-recently-used items.", "Cache invalidation is one of the hard problems.", "TTL-based expiry ensures eventual freshness.", "Write-through caches update both cache and DB.", "Cache warming pre-populates before traffic."},
		{"Message queues decouple producers and consumers.", "Dead letter queues capture failed messages.", "Backpressure prevents producer/consumer mismatch.", "At-least-once delivery may cause duplicates.", "Idempotent consumers handle duplicates safely."},
		{"JWT encodes claims as signed JSON.", "Refresh tokens outlive access tokens.", "OAuth 2.0 delegates authorization.", "MFA adds a factor beyond password.", "Session fixation sets a known ID pre-auth."},
		{"Inode-based cache keys survive renames.", "Atomic rename prevents partial writes.", "fsnotify uses inotify on Linux.", "Symlinks must be confined to prevent traversal.", "TUS enables resumable uploads."},
		{"FTS5 supports BM25 ranking.", "Trigrams allow substring matching.", "Stemming reduces words to root form.", "Query expansion adds synonyms.", "Faceted search filters by file type."},
		{"B-tree indexes support range queries.", "Covering indexes include all needed columns.", "Partial indexes keep index size small.", "Statistics help the planner choose access paths.", "Composite indexes order by column position."},
		{"Webhooks push events via HTTP POST.", "HMAC-SHA256 verifies authenticity.", "Exponential backoff handles transient failures.", "Idempotency keys prevent duplicate processing.", "Payloads include event type and timestamp."},
		{"Session tokens need 128+ bits of entropy.", "Sessions expire after TTL.", "Revoke sessions on password change.", "HttpOnly and Secure flags protect cookies.", "Bearer tokens skip CSRF checks."},
		{"Environment variables configure twelve-factor apps.", "Validate config at startup for fast failure.", "Never commit secrets to version control.", "Defaults should be safe for development.", "Schema documentation helps operators."},
		{"Multi-stage Docker builds shrink image size.", "Health check endpoints detect unhealthy pods.", "Graceful shutdown drains in-flight requests.", "Rolling deployments update one instance at a time.", "Blue-green allows instant rollback."},
		{"Structured logs enable machine querying.", "Metrics expose counters, gauges, histograms.", "Distributed tracing tracks cross-service requests.", "Alerts fire on threshold breaches.", "SLOs define acceptable error rates."},
		{"Log levels control verbosity.", "Correlation IDs link request log entries.", "Log rotation prevents disk exhaustion.", "Redact sensitive data before logging.", "slog in Go 1.21+ provides structured logging."},
		{"OpenTelemetry is the observability standard.", "Spans represent a unit of work.", "Context propagation passes trace IDs.", "Sampling reduces trace volume.", "Baggage carries values across a trace."},
		{"Prometheus scrapes metrics on schedule.", "Counters only increase; gauges go both ways.", "Histograms compute percentiles from buckets.", "Labels add filterable dimensions.", "High-cardinality labels degrade performance."},
		{"PagerDuty routes alerts to on-call.", "Alert fatigue degrades signal-to-noise.", "Runbooks guide incident response.", "Thresholds must exceed normal variance.", "Dead man's switch alerts fire on missing heartbeat."},
	}
	for i := 0; i < 120; i++ {
		topic := topics[i%len(topics)]
		pool := sentences[i%len(sentences)]
		var b strings.Builder
		fmt.Fprintf(&b, "Topic: %s\nEntry: %d\nCreated: %s\n\n", topic, i+1, time.Now().Format("2006-01-02"))
		for j := 0; j < 3+rng.Intn(4); j++ {
			b.WriteString(pool[rng.Intn(len(pool))] + "\n\n")
		}
		writeTxt(fmt.Sprintf("Large-Folder/%s-%03d.txt", topic, i+1), b.String())
	}
}

// ── file writers ──────────────────────────────────────────────────────────────

func writeFile(rel string, data []byte) {
	p := filepath.Join(root, rel)
	must(os.MkdirAll(filepath.Dir(p), 0o750))
	must(os.WriteFile(p, data, 0o640))
	fileCount++
	fmt.Printf("  [%3d] %s (%d B)\n", fileCount, rel, len(data))
}

func writeTxt(rel, body string) { writeFile(rel, []byte(body)) }

func writeMd(rel, title string, sections []string) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	for _, s := range sections {
		fmt.Fprintf(&b, "%s\n\n", s)
	}
	writeFile(rel, []byte(b.String()))
}

func writePDF(rel, title, body string) {
	safe := func(s string) string {
		return strings.NewReplacer("(", "[", ")", "]", "\n", " -- ").Replace(s)
	}
	var b bytes.Buffer
	nl := func(s string) { fmt.Fprintln(&b, s) }
	nl("%PDF-1.4")
	nl("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj")
	nl("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj")
	nl("3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj")
	stream := fmt.Sprintf("BT /F1 14 Tf 50 780 Td (%s) Tj 0 -30 Td /F1 11 Tf (%s) Tj ET", safe(title), safe(body[:min(len(body), 200)]))
	fmt.Fprintf(&b, "4 0 obj<</Length %d>>\nstream\n%s\nendstream endobj\n", len(stream), stream)
	nl("5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj")
	xref := b.Len()
	nl("xref\n0 6\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000266 00000 n \n0000000360 00000 n ")
	fmt.Fprintf(&b, "trailer<</Size 6/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", xref)
	writeFile(rel, b.Bytes())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── 3D formats ────────────────────────────────────────────────────────────────

type vec3 [3]float32
type tri struct{ n, v1, v2, v3 vec3 }

func writeSTL(rel string, tris []tri) {
	var b bytes.Buffer
	header := make([]byte, 80)
	copy(header, "goDrive test")
	b.Write(header)
	must(binary.Write(&b, binary.LittleEndian, uint32(len(tris))))
	for _, t := range tris {
		for _, v := range []vec3{t.n, t.v1, t.v2, t.v3} {
			for _, f := range v {
				must(binary.Write(&b, binary.LittleEndian, f))
			}
		}
		b.Write([]byte{0, 0})
	}
	writeFile(rel, b.Bytes())
}

func writePLY(rel string, n int) {
	var b strings.Builder
	fmt.Fprintf(&b, "ply\nformat ascii 1.0\nelement vertex %d\nproperty float x\nproperty float y\nproperty float z\nproperty uchar red\nproperty uchar green\nproperty uchar blue\nend_header\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%.4f %.4f %.4f %d %d %d\n",
			rng.Float32()*20-10, rng.Float32()*20-10, rng.Float32()*5,
			rng.Intn(256), rng.Intn(256), rng.Intn(256))
	}
	writeFile(rel, []byte(b.String()))
}

func write3MF(rel string) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		must(err)
		_, _ = w.Write([]byte(content))
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="model" ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Target="/3D/model.model" Id="r0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/></Relationships>`)
	add("3D/model.model", `<?xml version="1.0" encoding="UTF-8"?><model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"><resources><object id="1" type="model"><mesh><vertices><vertex x="0" y="0" z="0"/><vertex x="20" y="0" z="0"/><vertex x="20" y="20" z="0"/><vertex x="0" y="20" z="0"/><vertex x="0" y="0" z="20"/><vertex x="20" y="0" z="20"/><vertex x="20" y="20" z="20"/><vertex x="0" y="20" z="20"/></vertices><triangles><triangle v1="0" v2="1" v3="2"/><triangle v1="0" v2="2" v3="3"/><triangle v1="4" v2="6" v3="5"/><triangle v1="4" v2="7" v3="6"/><triangle v1="0" v2="4" v3="5"/><triangle v1="0" v2="5" v3="1"/><triangle v1="1" v2="5" v3="6"/><triangle v1="1" v2="6" v3="2"/><triangle v1="2" v2="6" v3="7"/><triangle v1="2" v2="7" v3="3"/><triangle v1="3" v2="7" v3="4"/><triangle v1="3" v2="4" v3="0"/></triangles></mesh></object></resources><build><item objectid="1"/></build></model>`)
	must(zw.Close())
	writeFile(rel, buf.Bytes())
}

func cylinderOBJ(segments int, r, h float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Cylinder r=%.1f h=%.1f\no Cylinder\n", r, h)
	for i := 0; i < segments; i++ {
		a := 2 * math.Pi * float64(i) / float64(segments)
		fmt.Fprintf(&b, "v %.4f %.4f 0\n", r*math.Cos(a), r*math.Sin(a))
	}
	for i := 0; i < segments; i++ {
		a := 2 * math.Pi * float64(i) / float64(segments)
		fmt.Fprintf(&b, "v %.4f %.4f %.4f\n", r*math.Cos(a), r*math.Sin(a), h)
	}
	fmt.Fprintf(&b, "v 0 0 0\nv 0 0 %.4f\n", h)
	bot, top := segments*2+1, segments*2+2
	for i := 0; i < segments; i++ {
		n := i%segments + 1
		fmt.Fprintf(&b, "f %d %d %d %d\n", i+1, n, n+segments, i+1+segments)
		fmt.Fprintf(&b, "f %d %d %d\n", bot, n, i+1)
		fmt.Fprintf(&b, "f %d %d %d\n", top, i+1+segments, n+segments)
	}
	return b.String()
}

// ── STL shape generators ──────────────────────────────────────────────────────

func boxTris(x, y, z, w, d, h float32) []tri {
	x2, y2, z2 := x+w, y+d, z+h
	faces := []struct {
		n  vec3
		vs [4]vec3
	}{
		{vec3{0, 0, 1}, [4]vec3{{x, y, z2}, {x2, y, z2}, {x2, y2, z2}, {x, y2, z2}}},
		{vec3{0, 0, -1}, [4]vec3{{x, y2, z}, {x2, y2, z}, {x2, y, z}, {x, y, z}}},
		{vec3{0, 1, 0}, [4]vec3{{x, y2, z}, {x, y2, z2}, {x2, y2, z2}, {x2, y2, z}}},
		{vec3{0, -1, 0}, [4]vec3{{x2, y, z}, {x2, y, z2}, {x, y, z2}, {x, y, z}}},
		{vec3{1, 0, 0}, [4]vec3{{x2, y, z}, {x2, y2, z}, {x2, y2, z2}, {x2, y, z2}}},
		{vec3{-1, 0, 0}, [4]vec3{{x, y, z2}, {x, y2, z2}, {x, y2, z}, {x, y, z}}},
	}
	var out []tri
	for _, f := range faces {
		out = append(out, tri{f.n, f.vs[0], f.vs[1], f.vs[2]}, tri{f.n, f.vs[0], f.vs[2], f.vs[3]})
	}
	return out
}

func cylTris(segs int, r, h float32) []tri {
	var out []tri
	for i := 0; i < segs; i++ {
		a0 := float64(i) / float64(segs) * 2 * math.Pi
		a1 := float64(i+1) / float64(segs) * 2 * math.Pi
		x0, y0 := float32(r)*float32(math.Cos(a0)), float32(r)*float32(math.Sin(a0))
		x1, y1 := float32(r)*float32(math.Cos(a1)), float32(r)*float32(math.Sin(a1))
		nx := float32((math.Cos(a0) + math.Cos(a1)) / 2)
		ny := float32((math.Sin(a0) + math.Sin(a1)) / 2)
		out = append(out,
			tri{vec3{nx, ny, 0}, vec3{x0, y0, 0}, vec3{x1, y1, 0}, vec3{x1, y1, h}},
			tri{vec3{nx, ny, 0}, vec3{x0, y0, 0}, vec3{x1, y1, h}, vec3{x0, y0, h}},
			tri{vec3{0, 0, -1}, vec3{0, 0, 0}, vec3{x1, y1, 0}, vec3{x0, y0, 0}},
			tri{vec3{0, 0, 1}, vec3{0, 0, h}, vec3{x0, y0, h}, vec3{x1, y1, h}},
		)
	}
	return out
}

func sphereTris(stacks, slices int, r float32) []tri {
	var out []tri
	for i := 0; i < stacks; i++ {
		phi0 := math.Pi * float64(i) / float64(stacks)
		phi1 := math.Pi * float64(i+1) / float64(stacks)
		for j := 0; j < slices; j++ {
			th0 := 2 * math.Pi * float64(j) / float64(slices)
			th1 := 2 * math.Pi * float64(j+1) / float64(slices)
			v := func(phi, th float64) vec3 {
				return vec3{
					r * float32(math.Sin(phi)*math.Cos(th)),
					r * float32(math.Sin(phi)*math.Sin(th)),
					r * float32(math.Cos(phi)),
				}
			}
			a, b2, c, d := v(phi0, th0), v(phi0, th1), v(phi1, th1), v(phi1, th0)
			n := vec3{(a[0] + b2[0] + c[0] + d[0]) / 4, (a[1] + b2[1] + c[1] + d[1]) / 4, (a[2] + b2[2] + c[2] + d[2]) / 4}
			out = append(out, tri{n, a, b2, c}, tri{n, a, c, d})
		}
	}
	return out
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
