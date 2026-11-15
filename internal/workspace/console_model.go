package workspace

import (
	"encoding/json"
	"os"
	"strings"
)

// Repair only the empty model emitted by an unconfigured initial deployment.
// Explicit user-selected models and provider configuration remain untouched.
func repairEmptyConsoleModel(configPath, settingsPath, model string) error {
	if model == "" {
		return nil
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	text := string(b)
	if strings.HasPrefix(text, "model = \"\"\n") {
		text = "model = " + tomlString(model) + "\n" + strings.TrimPrefix(text, "model = \"\"\n")
		if err = os.WriteFile(configPath, []byte(text), 0600); err != nil {
			return err
		}
	}
	b, err = os.ReadFile(settingsPath)
	if err != nil {
		return err
	}
	var settings map[string]json.RawMessage
	if err = json.Unmarshal(b, &settings); err != nil {
		return err
	}
	changed := false
	for _, key := range []string{"defaultModelSelection", "textGenerationModelSelection"} {
		var selection map[string]any
		if json.Unmarshal(settings[key], &selection) == nil && selection["instanceId"] == "codex" && selection["model"] == "" {
			selection["model"] = model
			settings[key], _ = json.Marshal(selection)
			changed = true
		}
	}
	if changed {
		b, err = json.Marshal(settings)
		if err != nil {
			return err
		}
		return os.WriteFile(settingsPath, b, 0600)
	}
	return nil
}
