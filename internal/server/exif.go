package server

import (
	"encoding/json"
	"net/http"
	"os/exec"

	"godrive/internal/preview"
	"godrive/internal/store"
)

type ExifData struct {
	Fields map[string]any `json:"fields"`
	GPSLat *float64       `json:"gps_lat,omitempty"`
	GPSLon *float64       `json:"gps_lon,omitempty"`
	HasGPS bool           `json:"has_gps"`
}

// exifSkipFields are noisy/redundant fields we strip before returning.
var exifSkipFields = map[string]bool{
	"SourceFile": true, "ExifToolVersion": true,
	"Directory": true, "FilePermissions": true,
	"FileAccessDate": true, "FileInodeChangeDate": true,
}

func (s *Server) fileExif(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	resolved, _, err := s.files.ResolveForRead(user, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	kind := preview.KindForName(resolved.Physical)
	if kind != "image" && kind != "raw" {
		writeError(w, http.StatusUnsupportedMediaType, "EXIF metadata only available for image and RAW files")
		return
	}

	path, err := exec.LookPath("exiftool")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "exiftool not available")
		return
	}

	out, err := exec.CommandContext(r.Context(), path,
		"-json", "-struct", "-n",
		"-coordFormat", "%.6f",
		resolved.Physical,
	).Output()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "exiftool failed")
		return
	}

	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
		writeError(w, http.StatusInternalServerError, "failed to parse exiftool output")
		return
	}

	raw := rows[0]
	fields := make(map[string]any, len(raw))
	for k, v := range raw {
		if !exifSkipFields[k] {
			fields[k] = v
		}
	}

	result := ExifData{Fields: fields}
	if lat, ok := raw["GPSLatitude"].(float64); ok {
		if lon, ok2 := raw["GPSLongitude"].(float64); ok2 {
			result.GPSLat = &lat
			result.GPSLon = &lon
			result.HasGPS = true
		}
	}

	writeJSON(w, http.StatusOK, result)
}
