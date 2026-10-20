package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/opencode-ai/opencode/internal/logging"
)

// configFile is one discovered configuration file together with the directory it
// lives in (used to resolve relative "{file:...}" references).
type configFile struct {
	path string
	dir  string
}

// v2ConfigFilenames are the v2 configuration file names, most specific first.
var v2ConfigFilenames = []string{"opencode.json", "opencode.jsonc"}

// legacyConfigFilename is the pre-v2 single-file name, still read for
// back-compatibility.
const legacyConfigFilename = ".opencode.json"

// configFiles returns every configuration file to load, in precedence order
// (lowest first; later files override earlier ones). It covers both the v2
// layout (opencode.json/opencode.jsonc) and the legacy ".opencode.json":
//
// v2 precedence (Spec 11a, low -> high):
//  1. global ~/.config/opencode/opencode.json(c)
//  2. opencode.json(c) from the filesystem root down to the working directory
//  3. .opencode/opencode.json(c) (wins over the direct file at the same level)
//  4. /etc/opencode/opencode.json(c)
//
// The legacy ".opencode.json" files are read at the same slots as their v2
// counterparts: the global ones (in viper's old search paths) before the v2
// ancestor walk, and the project-level one right before the working directory's
// v2 files. This keeps existing project configs winning over global settings
// while a project v2 file still overrides the legacy project file.
func configFiles(workingDir string) []configFile {
	var files []configFile

	home, _ := os.UserHomeDir()

	// 1. Global legacy + v2 config.
	files = append(files, legacyGlobalFiles(home)...)
	if home != "" {
		files = append(files, filesIn(filepath.Join(home, ".config", appName))...)
	}

	// 2. Direct opencode.json(c), from the farthest ancestor to workingDir.
	// 3. .opencode/opencode.json(c) at each level (added right after its level).
	// The project-level legacy ".opencode.json" is read at the working directory
	// slot, before that level's v2 files.
	wd := filepath.Clean(workingDir)
	ancestors := ancestorsRootFirst(wd)
	for _, dir := range ancestors {
		if dir == wd {
			if legacy := filepath.Join(dir, legacyConfigFilename); isFile(legacy) {
				files = append(files, configFile{path: legacy, dir: dir})
			}
		}
		files = append(files, filesIn(dir)...)
		files = append(files, filesIn(filepath.Join(dir, defaultDataDirectory))...)
	}

	// 4. System-wide config wins over everything else.
	files = append(files, filesIn(filepath.Join(string(filepath.Separator), "etc", appName))...)

	return files
}

// legacyGlobalFiles returns the legacy ".opencode.json" from viper's historical
// global search paths, lowest precedence first.
func legacyGlobalFiles(home string) []configFile {
	var files []configFile
	add := func(dir string) {
		if dir == "" {
			return
		}
		p := filepath.Join(dir, legacyConfigFilename)
		if isFile(p) {
			files = append(files, configFile{path: p, dir: dir})
		}
	}
	add(home)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		add(filepath.Join(xdg, appName))
	} else if home != "" {
		add(filepath.Join(home, ".config", appName))
	}
	return files
}

// filesIn returns the v2 config files that exist in dir, in name order.
func filesIn(dir string) []configFile {
	var out []configFile
	for _, name := range v2ConfigFilenames {
		p := filepath.Join(dir, name)
		if isFile(p) {
			out = append(out, configFile{path: p, dir: dir})
		}
	}
	return out
}

// ancestorsRootFirst returns dir and each of its ancestors up to the filesystem
// root, ordered from the root down to dir (so the deepest, most specific
// directory comes last).
func ancestorsRootFirst(dir string) []string {
	dir = filepath.Clean(dir)
	var reversed []string
	for {
		reversed = append(reversed, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	out := make([]string, len(reversed))
	for i, d := range reversed {
		out[len(reversed)-1-i] = d
	}
	return out
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// readConfigMaps reads every configuration file in precedence order, tolerating
// JSONC, and merges them into a single settings map (later files win). Every
// string value is interpolated ({env:}/{file:}) relative to the directory of the
// file that declared it, so relative "{file:...}" references resolve next to
// their config file (Spec 11a). A file that fails to parse is skipped with a
// warning rather than aborting the whole load.
func readConfigMaps(workingDir string) map[string]any {
	merged := map[string]any{}
	for _, file := range configFiles(workingDir) {
		data, err := os.ReadFile(file.path)
		if err != nil {
			logging.Warn("failed to read config file", "file", file.path, "error", err)
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(stripJsonc(string(data))), &m); err != nil {
			logging.Warn("failed to parse config file", "file", file.path, "error", err)
			continue
		}
		merged = mergeMaps(merged, interpolateMap(m, file.dir).(map[string]any))
	}
	return merged
}

// interpolateMap applies interpolate to every string in a decoded JSON value
// (maps, slices and scalars), returning a value of the same shape.
func interpolateMap(v any, baseDir string) any {
	switch t := v.(type) {
	case string:
		return interpolate(t, baseDir)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = interpolateMap(e, baseDir)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = interpolateMap(e, baseDir)
		}
		return out
	default:
		return v
	}
}

// mergeMaps deep-merges src into dst (values in src win). Nested maps are merged
// recursively; any other value replaces the previous one.
func mergeMaps(dst, src map[string]any) map[string]any {
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				dst[k] = mergeMaps(dm, sm)
				continue
			}
		}
		dst[k] = sv
	}
	return dst
}
