package seeder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const sectionIndexCacheVersion = 1

type sectionIndexCache struct {
	Version     int              `json:"version"`
	Size        int64            `json:"size"`
	ModTimeUnix int64            `json:"mod_time_unix"`
	Sections    map[string]int64 `json:"sections"`
}

func sectionIndexCachePath(jsonPath string) string {
	return jsonPath + ".section-index"
}

func loadOrBuildSectionIndex(jsonPath string) (map[string]int64, bool, error) {
	info, err := os.Stat(jsonPath)
	if err != nil {
		return nil, false, err
	}

	if index, ok := loadSectionIndexCache(jsonPath, info); ok {
		return index, true, nil
	}

	index, err := buildSectionIndex(jsonPath)
	if err != nil {
		return nil, false, err
	}
	if err := saveSectionIndexCache(jsonPath, info, index); err != nil {
		return index, false, fmt.Errorf("save section index cache: %w", err)
	}
	return index, false, nil
}

func loadSectionIndexCache(jsonPath string, info os.FileInfo) (map[string]int64, bool) {
	path := sectionIndexCachePath(jsonPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cached sectionIndexCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, false
	}
	if cached.Version != sectionIndexCacheVersion ||
		cached.Size != info.Size() ||
		cached.ModTimeUnix != info.ModTime().Unix() {
		return nil, false
	}
	if len(cached.Sections) == 0 {
		return nil, false
	}
	return cached.Sections, true
}

func saveSectionIndexCache(jsonPath string, info os.FileInfo, index map[string]int64) error {
	path := sectionIndexCachePath(jsonPath)
	cached := sectionIndexCache{
		Version:     sectionIndexCacheVersion,
		Size:        info.Size(),
		ModTimeUnix: info.ModTime().Unix(),
		Sections:    index,
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
