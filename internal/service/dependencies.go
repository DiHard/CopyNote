package service

import (
	"time"

	"copynote/internal/autorun"
	"copynote/internal/clipboard"
	"copynote/internal/storage"
)

// Keep this linker variable in service: test-instance builds override its exact
// path (-X copynote/internal/service.autorunValueName=...) to protect real autorun.
var autorunValueName = "CopyNote"

const AutostartFlag = autorun.AutostartFlag

func withDefaults(deps Dependencies) Dependencies {
	if deps.LoadStore == nil {
		deps.LoadStore = storage.Load
	}
	if deps.SaveStore == nil {
		deps.SaveStore = storage.Save
	}
	if deps.LoadLegacySettings == nil {
		deps.LoadLegacySettings = storage.LoadLegacySettings
	}
	if deps.WriteText == nil {
		deps.WriteText = clipboard.WriteText
	}
	if deps.SetAutorun == nil {
		deps.SetAutorun = func(enabled bool) error { return autorun.SetEnabled(autorunValueName, enabled) }
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	return deps
}
