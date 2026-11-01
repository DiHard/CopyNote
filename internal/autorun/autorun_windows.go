package autorun

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

const keyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

func SetEnabled(valueName string, enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }() // Closing the handle does not change the completed registry operation.
	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return k.SetStringValue(valueName, `"`+exe+`" `+AutostartFlag)
	}
	err = k.DeleteValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
