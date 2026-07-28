package config

import (
	"encoding/json"

	"tdl-ui/internal/logging"
)

var logStore = logging.L("config")

// legacyJSONUnmarshal 解析旧版 settings.json（扁平结构，含 JSON 标签）。
func legacyJSONUnmarshal(b []byte, s *Settings) error {
	return json.Unmarshal(b, s)
}
